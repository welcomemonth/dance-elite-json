// Package wxpay 封装微信支付 V3 下单与小程序支付参数签名（开发计划 T10）。
//
// 双通道（R2 定稿：mock 优先，证书到位后切真实通道，小程序端不感知）：
//   - Client：真实微信支付 V3（JSAPI 下单 + SHA256-RSA 签名），证书/私钥
//     经环境变量注入（configs，T3 密钥管理）；
//   - MockGateway：无商户证书时的 fake 通道，供本地与联调环境拉起"支付"。
//
// 安全约定：日志中绝不打印商户私钥、APIv3 密钥与请求/响应体之外的敏感内容；
// 本包不感知订单业务，只接受 out_trade_no/金额/openid 等下单参数。
package wxpay

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Gateway 是微信支付下单网关抽象：真实 V3 客户端与 mock 通道共用，
// service 层依赖本接口，测试注入 fake。
type Gateway interface {
	// Prepay 调微信统一下单（JSAPI），返回 prepay_id。
	// 微信侧业务错误返回 *Error；网络错误为普通 error（调用方可重试语义区分）。
	Prepay(ctx context.Context, req PrepayRequest) (string, error)
	// SignPayParams 用商户私钥生成小程序支付参数（paySign 为 SHA256-RSA 签名）。
	SignPayParams(prepayID string) (PayParams, error)
}

// PrepayRequest 是一次统一下单的参数（与订单业务解耦）。
type PrepayRequest struct {
	OpenID      string // 支付用户在小程序 appid 下的 openid（JSAPI 必填）
	OutTradeNo  string // 商户订单号（全局唯一，不可推测，T9 生成规则）
	Description string // 商品描述（展示在用户支付记录中）
	AmountFen   int64  // 金额（分）
}

// PayParams 是小程序 wx.requestPayment 所需参数（API 文档 3.1b pay_params）。
// 字段名为微信协议约定的 camelCase，是项目 snake_case 规则的例外。
type PayParams struct {
	TimeStamp string
	NonceStr  string
	Package   string
	SignType  string
	PaySign   string
}

// Error 是微信支付返回的业务错误（HTTP 非 2xx 且按微信错误协议解析）。
// 网络/传输层错误不是本类型。
type Error struct {
	Status  int    // HTTP 状态码
	Code    string // 微信错误码（如 INVALID_REQUEST、OUT_TRADE_NO_USED）
	Message string // 微信错误描述
}

func (e *Error) Error() string {
	return fmt.Sprintf("wxpay: %s (code=%s, status=%d)", e.Message, e.Code, e.Status)
}

// defaultBaseURL 是微信支付 V3 生产地址（可在 Client 上替换，测试用 httptest）。
const defaultBaseURL = "https://api.mch.weixin.qq.com"

// jsapiPath 是 JSAPI 统一下单路径。
const jsapiPath = "/v3/pay/transactions/jsapi"

// maxHTTPRetries 是传输层失败（网络错误/超时）的最大重试次数（不含首次）。
const maxHTTPRetries = 1

// Client 是微信支付 V3 真实通道。构造时注入商户配置；私钥只存内存。
type Client struct {
	appID      string
	mchID      string
	serialNo   string
	privateKey *rsa.PrivateKey
	notifyURL  string

	// 回调验签与解密（T11）：平台证书公钥 + APIv3 密钥。
	apiV3Key          string
	platformPublicKey *rsa.PublicKey
	platformSerial    string

	httpClient *http.Client
	baseURL    string
	now        func() time.Time
}

// ClientOptions 是微信支付 V3 真实通道的商户配置，全部来自环境变量（T3）。
type ClientOptions struct {
	AppID           string
	MchID           string
	SerialNo        string // 商户 API 证书序列号（请求签名用）
	PrivateKeyPEM   string // 商户 API 私钥（PKCS#8 / PKCS#1 PEM）
	APIv3Key        string // APIv3 密钥，32 字节，回调报文 AES-256-GCM 解密
	PlatformCertPEM string // 微信支付平台证书 PEM，回调验签
	NotifyURL       string // 支付结果回调地址（POST /api/v1/payments/wechat/notify）
}

// NewClient 构造真实微信支付客户端；PEM 支持 PKCS#8 / PKCS#1 与平台证书，
// 环境变量中的 \n 字面量会自动还原为换行。任何解析失败返回错误，由组合根
// fail-fast（密钥缺失或格式错误不进入运行态）。
func NewClient(opts ClientOptions) (*Client, error) {
	key, err := parsePrivateKey(opts.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("解析微信支付商户私钥: %w", err)
	}
	if len(opts.APIv3Key) != 32 {
		return nil, fmt.Errorf("微信支付 APIv3 密钥必须为 32 字节，当前 %d 字节", len(opts.APIv3Key))
	}
	platformKey, platformSerial, err := parsePlatformCert(opts.PlatformCertPEM)
	if err != nil {
		return nil, fmt.Errorf("解析微信支付平台证书: %w", err)
	}
	return &Client{
		appID:             opts.AppID,
		mchID:             opts.MchID,
		serialNo:          opts.SerialNo,
		privateKey:        key,
		notifyURL:         opts.NotifyURL,
		apiV3Key:          opts.APIv3Key,
		platformPublicKey: platformKey,
		platformSerial:    platformSerial,
		httpClient:        &http.Client{Timeout: 10 * time.Second},
		baseURL:           defaultBaseURL,
		now:               time.Now,
	}, nil
}

// Prepay 调微信 JSAPI 统一下单（传输层失败重试一次，微信业务错误不重试）。
func (c *Client) Prepay(ctx context.Context, req PrepayRequest) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= maxHTTPRetries; attempt++ {
		prepayID, err := c.doPrepay(ctx, req)
		if err == nil {
			return prepayID, nil
		}
		var wechatErr *Error
		if errors.As(err, &wechatErr) {
			return "", err // 微信已受理并拒绝：重试无意义
		}
		lastErr = err
	}
	return "", fmt.Errorf("wxpay prepay: %w", lastErr)
}

// doPrepay 执行一次下单请求并解析响应。
func (c *Client) doPrepay(ctx context.Context, req PrepayRequest) (string, error) {
	body, err := json.Marshal(map[string]any{
		"appid":        c.appID,
		"mchid":        c.mchID,
		"description":  req.Description,
		"out_trade_no": req.OutTradeNo,
		"notify_url":   c.notifyURL,
		"amount": map[string]any{
			"currency": "CNY",
			"total":    req.AmountFen,
		},
		"payer": map[string]any{
			"openid": req.OpenID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal jsapi body: %w", err)
	}

	timestamp, nonce, signature, err := c.signRequest(http.MethodPost, jsapiPath, body)
	if err != nil {
		return "", err
	}
	authorization := c.authorizationHeader(timestamp, nonce, signature)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+jsapiPath, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build jsapi request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", authorization)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("post jsapi: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("read jsapi response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", parseWechatError(resp.StatusCode, respBody)
	}

	var ok struct {
		PrepayID string `json:"prepay_id"`
	}
	if err := json.Unmarshal(respBody, &ok); err != nil || ok.PrepayID == "" {
		return "", fmt.Errorf("parse jsapi response: %w", err)
	}
	return ok.PrepayID, nil
}

// SignPayParams 生成小程序支付参数：paySign 对
// "appId\ntimeStamp\nnonceStr\npackage\n" 做 SHA256-RSA 签名。
func (c *Client) SignPayParams(prepayID string) (PayParams, error) {
	timestamp := strconv.FormatInt(c.now().Unix(), 10)
	nonce, err := randomHex(16)
	if err != nil {
		return PayParams{}, err
	}
	pkg := "prepay_id=" + prepayID
	message := c.appID + "\n" + timestamp + "\n" + nonce + "\n" + pkg + "\n"
	signature, err := c.signMessage([]byte(message))
	if err != nil {
		return PayParams{}, err
	}
	return PayParams{
		TimeStamp: timestamp,
		NonceStr:  nonce,
		Package:   pkg,
		SignType:  "RSA",
		PaySign:   signature,
	}, nil
}

// signMessage 对消息做 SHA256-RSA（PKCS#1 v1.5）签名并返回 base64。
func (c *Client) signMessage(message []byte) (string, error) {
	digest := sha256.Sum256(message)
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign message: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// parsePrivateKey 解析 PKCS#8 / PKCS#1 PEM 私钥；环境变量中的字面量 "\n"
// 还原为真实换行（godotenv 单行值常见写法）。
func parsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	normalized := strings.ReplaceAll(raw, `\n`, "\n")
	block, _ := pem.Decode([]byte(normalized))
	if block == nil {
		return nil, errors.New("PEM 块缺失（检查 WECHAT_PAY_PRIVATE_KEY 是否完整）")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("私钥不是 RSA 密钥")
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("无法解析私钥（支持 PKCS#8 / PKCS#1 PEM）")
}

// randomHex 返回 n 字节随机数的十六进制串。
func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// MockGateway 是 fake 支付通道（R2：PAY_MOCK=true 时使用）。
// Prepay 恒成功并返回 mock-prepay-* 单号；签名可验证但无密码学意义，
// 仅用于本地与联调环境走通"下单 → 拉起支付"链路，小程序端不感知。
type MockGateway struct {
	now func() time.Time
}

// NewMockGateway 构造 mock 支付通道。
func NewMockGateway() *MockGateway {
	return &MockGateway{now: time.Now}
}

// Prepay 返回 fake 预支付单号。
func (m *MockGateway) Prepay(_ context.Context, _ PrepayRequest) (string, error) {
	suffix, err := randomHex(8)
	if err != nil {
		return "", err
	}
	return "mock-prepay-" + suffix, nil
}

// SignPayParams 返回结构完整的假支付参数（paySign 为 "MOCK-SIGN" 标记）。
func (m *MockGateway) SignPayParams(prepayID string) (PayParams, error) {
	nonce, err := randomHex(16)
	if err != nil {
		return PayParams{}, err
	}
	return PayParams{
		TimeStamp: strconv.FormatInt(m.now().Unix(), 10),
		NonceStr:  nonce,
		Package:   "prepay_id=" + prepayID,
		SignType:  "RSA",
		PaySign:   "MOCK-SIGN",
	}, nil
}
