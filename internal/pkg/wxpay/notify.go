package wxpay

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidNotify 表示微信支付回调验签/解密/报文校验失败。
// 路由层据此返回 HTTP 401，微信会按协议重推（API 文档 3.2）。
var ErrInvalidNotify = errors.New("wxpay: invalid notify")

// 微信支付交易状态（trade_state）中本项目关心的取值。
// CLOSED/REVOKED/PAYERROR 属于"不会再有成功支付"的终态，T12 据此把
// 待支付订单取消并释放名额；NOTPAY 仍可能完成支付，不做取消。
const (
	TradeStateSuccess  = "SUCCESS"
	TradeStateNotPay   = "NOTPAY"
	TradeStateClosed   = "CLOSED"
	TradeStateRevoked  = "REVOKED"
	TradeStatePayError = "PAYERROR"
)

// IsFailureTradeState 判断交易是否已失败/关闭（可安全取消待支付订单）。
func IsFailureTradeState(state string) bool {
	switch state {
	case TradeStateClosed, TradeStateRevoked, TradeStatePayError:
		return true
	default:
		return false
	}
}

// notifyMaxSkew 是回调时间戳允许的最大偏差（微信官方建议 5 分钟），
// 超出视为重放/过期请求拒绝。
const notifyMaxSkew = 5 * time.Minute

// NotifyHeaders 是微信支付回调的签名头（API 文档 3.2）。
type NotifyHeaders struct {
	Timestamp string // Wechatpay-Timestamp
	Nonce     string // Wechatpay-Nonce
	Signature string // Wechatpay-Signature
	Serial    string // Wechatpay-Serial（平台证书序列号）
}

// Transaction 是回调解密后或主动查单得到的交易结果。
// 只保留本项目需要的字段，敏感内容不扩展。
type Transaction struct {
	OutTradeNo    string // 商户订单号（orders.order_no）
	TransactionID string // 微信支付订单号（payments.transaction_id 幂等键）
	TradeState    string // SUCCESS / NOTPAY / CLOSED ...
	AmountTotal   int64  // 实付金额（分）
	SuccessTime   string // 支付成功时间（RFC3339，微信原样）
	AppID         string
	MchID         string
}

// Notifier 解析并验签微信支付回调（真实通道：验签 + AES-256-GCM 解密；
// mock 通道：明文交易 JSON）。service 层依赖本接口，测试注入 fake。
type Notifier interface {
	ParseNotify(headers NotifyHeaders, body []byte) (*Transaction, error)
}

// OrderQuerier 是主动查单能力（T11 回调丢失兜底）。
type OrderQuerier interface {
	QueryOrder(ctx context.Context, outTradeNo string) (*Transaction, error)
}

// Provider 是支付网关的完整能力集合：预下单 + 回调解析 + 主动查单。
// 真实通道 Client 与 MockGateway 均实现，组合根据此一次装配。
type Provider interface {
	Gateway
	Notifier
	OrderQuerier
}

// ParseNotify 验签并解密微信支付回调，返回交易结果。
// 任一环节失败都包装 ErrInvalidNotify（路由层映射 HTTP 401）。
func (c *Client) ParseNotify(headers NotifyHeaders, body []byte) (*Transaction, error) {
	if err := c.verifyNotifySignature(headers, body); err != nil {
		return nil, err
	}

	var envelope notifyEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("%w: 回调报文不是合法 JSON", ErrInvalidNotify)
	}
	if envelope.Resource.Algorithm != "AEAD_AES_256_GCM" {
		return nil, fmt.Errorf("%w: 不支持的加密算法 %q", ErrInvalidNotify, envelope.Resource.Algorithm)
	}
	plaintext, err := decryptNotifyResource(c.apiV3Key, envelope.Resource)
	if err != nil {
		return nil, fmt.Errorf("%w: 解密回调报文失败", ErrInvalidNotify)
	}
	tx, err := parseTransaction(plaintext)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidNotify, err)
	}
	if err := validateSuccessTransaction(tx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidNotify, err)
	}
	if c.appID != "" && tx.AppID != "" && tx.AppID != c.appID {
		return nil, fmt.Errorf("%w: appid 不匹配", ErrInvalidNotify)
	}
	if c.mchID != "" && tx.MchID != "" && tx.MchID != c.mchID {
		return nil, fmt.Errorf("%w: mchid 不匹配", ErrInvalidNotify)
	}
	return tx, nil
}

// verifyNotifySignature 校验时间戳新鲜度、平台证书序列号与 RSA 签名。
func (c *Client) verifyNotifySignature(headers NotifyHeaders, body []byte) error {
	ts, err := strconv.ParseInt(headers.Timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: 时间戳不合法", ErrInvalidNotify)
	}
	skew := c.now().Sub(time.Unix(ts, 0))
	if skew > notifyMaxSkew || skew < -notifyMaxSkew {
		return fmt.Errorf("%w: 回调时间戳超出允许偏差", ErrInvalidNotify)
	}
	if !strings.EqualFold(strings.TrimSpace(headers.Serial), c.platformSerial) {
		return fmt.Errorf("%w: 平台证书序列号不匹配", ErrInvalidNotify)
	}
	signature, err := base64.StdEncoding.DecodeString(headers.Signature)
	if err != nil {
		return fmt.Errorf("%w: 签名不是合法 base64", ErrInvalidNotify)
	}
	message := headers.Timestamp + "\n" + headers.Nonce + "\n" + string(body) + "\n"
	digest := sha256.Sum256([]byte(message))
	if err := rsa.VerifyPKCS1v15(c.platformPublicKey, crypto.SHA256, digest[:], signature); err != nil {
		return fmt.Errorf("%w: 签名验证失败", ErrInvalidNotify)
	}
	return nil
}

// QueryOrder 调微信支付查单接口（按商户订单号），供回调丢失时兜底。
// 微信侧业务错误返回 *Error，网络错误为普通 error。
func (c *Client) QueryOrder(ctx context.Context, outTradeNo string) (*Transaction, error) {
	path := "/v3/pay/transactions/out-trade-no/" + url.PathEscape(outTradeNo)
	query := "?mchid=" + url.QueryEscape(c.mchID)
	canonical := path + query

	timestamp, nonce, signature, err := c.signRequest(http.MethodGet, canonical, nil)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+canonical, nil)
	if err != nil {
		return nil, fmt.Errorf("build query request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", c.authorizationHeader(timestamp, nonce, signature))

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("query transaction %q: %w", outTradeNo, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("read query response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseWechatError(resp.StatusCode, respBody)
	}
	tx, err := parseTransaction(respBody)
	if err != nil {
		return nil, fmt.Errorf("parse query transaction %q: %w", outTradeNo, err)
	}
	return tx, nil
}

// notifyEnvelope 是回调外层报文（resource 为密文信封）。
type notifyEnvelope struct {
	Resource notifyResource `json:"resource"`
}

// notifyResource 是微信支付回调的加密资源信封。
type notifyResource struct {
	Algorithm      string `json:"algorithm"`
	Ciphertext     string `json:"ciphertext"`
	AssociatedData string `json:"associated_data"`
	Nonce          string `json:"nonce"`
	OriginalType   string `json:"original_type"`
}

// decryptNotifyResource 用 APIv3 密钥做 AES-256-GCM 解密。
func decryptNotifyResource(apiV3Key string, resource notifyResource) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(resource.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	block, err := aes.NewCipher([]byte(apiV3Key))
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	plaintext, err := gcm.Open(nil, []byte(resource.Nonce), ciphertext, []byte(resource.AssociatedData))
	if err != nil {
		return nil, fmt.Errorf("gcm open: %w", err)
	}
	return plaintext, nil
}

// transactionPayload 是解密后/查单响应中的交易 JSON（只取所需字段）。
type transactionPayload struct {
	AppID         string `json:"appid"`
	MchID         string `json:"mchid"`
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
	TradeState    string `json:"trade_state"`
	SuccessTime   string `json:"success_time"`
	Amount        struct {
		Total    int64  `json:"total"`
		Currency string `json:"currency"`
	} `json:"amount"`
}

// parseTransaction 解析交易 JSON 并校验基础字段与币种。
// 未支付订单的查单响应可能没有 transaction_id，故此处不强制要求
// （成功交易在 validateSuccessTransaction 中单独校验）。
func parseTransaction(raw []byte) (*Transaction, error) {
	var payload transactionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("解析交易报文失败: %w", err)
	}
	if payload.OutTradeNo == "" || payload.TradeState == "" {
		return nil, errors.New("交易报文缺少 out_trade_no/trade_state")
	}
	if payload.Amount.Currency != "" && payload.Amount.Currency != "CNY" {
		return nil, fmt.Errorf("不支持的币种 %q", payload.Amount.Currency)
	}
	return &Transaction{
		OutTradeNo:    payload.OutTradeNo,
		TransactionID: payload.TransactionID,
		TradeState:    payload.TradeState,
		AmountTotal:   payload.Amount.Total,
		SuccessTime:   payload.SuccessTime,
		AppID:         payload.AppID,
		MchID:         payload.MchID,
	}, nil
}

// validateSuccessTransaction 校验成功交易必须带微信支付订单号
// （payments.transaction_id 幂等键，缺失则无法入账）。
func validateSuccessTransaction(tx *Transaction) error {
	if tx.TradeState == TradeStateSuccess && tx.TransactionID == "" {
		return errors.New("成功交易缺少 transaction_id")
	}
	return nil
}

// signRequest 生成请求签名（微信支付 V3：METHOD\nURL\n时间戳\n随机串\nbody\n）。
func (c *Client) signRequest(method, canonicalURL string, body []byte) (timestamp, nonce, signature string, err error) {
	timestamp = strconv.FormatInt(c.now().Unix(), 10)
	nonce, err = randomHex(16)
	if err != nil {
		return "", "", "", err
	}
	message := method + "\n" + canonicalURL + "\n" + timestamp + "\n" + nonce + "\n" + string(body) + "\n"
	signature, err = c.signMessage([]byte(message))
	if err != nil {
		return "", "", "", err
	}
	return timestamp, nonce, signature, nil
}

// authorizationHeader 组装微信支付 V3 Authorization 头。
func (c *Client) authorizationHeader(timestamp, nonce, signature string) string {
	return fmt.Sprintf(
		`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",signature="%s",timestamp="%s",serial_no="%s"`,
		c.mchID, nonce, signature, timestamp, c.serialNo)
}

// parseWechatError 把微信支付非 2xx 响应解析为 *Error。
func parseWechatError(status int, body []byte) *Error {
	var wxErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &wxErr); err != nil || wxErr.Code == "" {
		return &Error{Status: status, Code: "UNKNOWN", Message: "微信支付返回异常"}
	}
	return &Error{Status: status, Code: wxErr.Code, Message: wxErr.Message}
}

// parsePlatformCert 解析微信支付平台证书 PEM，返回公钥与序列号（大写十六进制，
// 与回调头 Wechatpay-Serial 口径一致）。
func parsePlatformCert(raw string) (*rsa.PublicKey, string, error) {
	normalized := strings.ReplaceAll(raw, `\n`, "\n")
	block, _ := pem.Decode([]byte(normalized))
	if block == nil {
		return nil, "", errors.New("PEM 块缺失（检查 WECHAT_PAY_CERT 是否完整）")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("解析平台证书: %w", err)
	}
	publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, "", errors.New("平台证书公钥不是 RSA")
	}
	return publicKey, strings.ToUpper(cert.SerialNumber.Text(16)), nil
}

// ParseNotify 是 mock 通道的回调解析：直接读取明文交易 JSON，不做验签解密
// （PAY_MOCK=true 仅供本地/联调，生产关闭）。报文格式与解密后的 resource 一致。
func (m *MockGateway) ParseNotify(_ NotifyHeaders, body []byte) (*Transaction, error) {
	tx, err := parseTransaction(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidNotify, err)
	}
	if err := validateSuccessTransaction(tx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidNotify, err)
	}
	return tx, nil
}

// QueryOrder 是 mock 通道的查单：恒返回未支付（mock 下没有真实支付流水；
// 联调支付成功请直接 POST 明文回调报文）。
func (m *MockGateway) QueryOrder(_ context.Context, outTradeNo string) (*Transaction, error) {
	return &Transaction{OutTradeNo: outTradeNo, TradeState: TradeStateNotPay}, nil
}

// 编译期断言：两种通道都实现完整支付能力。
var (
	_ Provider = (*Client)(nil)
	_ Provider = (*MockGateway)(nil)
)
