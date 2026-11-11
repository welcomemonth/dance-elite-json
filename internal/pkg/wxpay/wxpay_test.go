package wxpay

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testKey 生成测试用 RSA 密钥对并返回 PKCS#8 PEM 文本。
func testKey(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	return pemText, key
}

// 构造一个自带证书的 Client（serial 取自自签证书），便于验签。
func testClient(t *testing.T, keyPEM string, key *rsa.PrivateKey) *Client {
	t.Helper()
	// 自签证书只为拿到一个"合法"序列号；客户端不校验证书内容。
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(123456), Subject: pkix.Name{CommonName: "test"}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	client, err := NewClient("wx-appid", "1900000109", cert.SerialNumber.String(), keyPEM, "https://example.com/pay/callback")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.now = func() time.Time { return time.Unix(1767221400, 0).UTC() }
	return client
}

func TestSignPayParams_Verifiable(t *testing.T) {
	keyPEM, key := testKey(t)
	client := testClient(t, keyPEM, key)

	params, err := client.SignPayParams("wx-prepay-123")
	if err != nil {
		t.Fatalf("SignPayParams: %v", err)
	}
	if params.SignType != "RSA" || params.Package != "prepay_id=wx-prepay-123" ||
		params.TimeStamp != "1767221400" || params.NonceStr == "" || params.PaySign == "" {
		t.Fatalf("params = %+v", params)
	}

	// 用公钥验签 "appId\ntimeStamp\nnonceStr\npackage\n"。
	message := "wx-appid\n" + params.TimeStamp + "\n" + params.NonceStr + "\n" + params.Package + "\n"
	digest := sha256.Sum256([]byte(message))
	sig, err := base64.StdEncoding.DecodeString(params.PaySign)
	if err != nil {
		t.Fatalf("decode paySign: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("verify paySign: %v", err)
	}
}

func TestPrepay_SuccessAndSignature(t *testing.T) {
	keyPEM, key := testKey(t)

	var gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != jsapiPath {
			t.Errorf("path = %s, want %s", r.URL.Path, jsapiPath)
		}
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"prepay_id":"wx-test-prepay-1"}`))
	}))
	defer server.Close()

	client := testClient(t, keyPEM, key)
	client.baseURL = server.URL
	prepayID, err := client.Prepay(context.Background(), PrepayRequest{
		OpenID:      "o-openid",
		OutTradeNo:  "2027010109300012345678",
		Description: "优舞盟报名费-测试赛",
		AmountFen:   98000,
	})
	if err != nil {
		t.Fatalf("Prepay: %v", err)
	}
	if prepayID != "wx-test-prepay-1" {
		t.Fatalf("prepay_id = %q", prepayID)
	}

	// 请求体字段。
	for _, want := range []string{`"out_trade_no":"2027010109300012345678"`, `"total":98000`, `"openid":"o-openid"`,
		`"notify_url":"https://example.com/pay/callback"`, `"mchid":"1900000109"`, `"appid":"wx-appid"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("body %s should contain %s", gotBody, want)
		}
	}

	// Authorization 头与签名验签。
	if !strings.HasPrefix(gotAuth, "WECHATPAY2-SHA256-RSA2048 ") {
		t.Fatalf("authorization = %q", gotAuth)
	}
	headers := parseAuthParams(t, gotAuth)
	if headers["mchid"] != "1900000109" || headers["timestamp"] != "1767221400" || headers["nonce_str"] == "" {
		t.Fatalf("auth params = %+v", headers)
	}
	signMessage := "POST\n" + jsapiPath + "\n" + headers["timestamp"] + "\n" + headers["nonce_str"] + "\n" + gotBody + "\n"
	digest := sha256.Sum256([]byte(signMessage))
	sig, err := base64.StdEncoding.DecodeString(headers["signature"])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("verify authorization signature: %v", err)
	}
}

func TestPrepay_WechatError(t *testing.T) {
	keyPEM, key := testKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"OUT_TRADE_NO_USED","message":"订单号已存在"}`))
	}))
	defer server.Close()

	client := testClient(t, keyPEM, key)
	client.baseURL = server.URL
	_, err := client.Prepay(context.Background(), PrepayRequest{OutTradeNo: "dup", AmountFen: 1})
	var wxErr *Error
	if !errors.As(err, &wxErr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if wxErr.Code != "OUT_TRADE_NO_USED" || wxErr.Status != http.StatusBadRequest {
		t.Fatalf("wxErr = %+v", wxErr)
	}
}

// flakyTransport 前 N 次返回网络错误，之后转交真实 transport。
type flakyTransport struct {
	failures int
	calls    int
	base     http.RoundTripper
}

func (f *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.calls++
	if f.calls <= f.failures {
		return nil, fmt.Errorf("network broken (test)")
	}
	return f.base.RoundTrip(req)
}

func TestPrepay_NetworkRetry(t *testing.T) {
	keyPEM, key := testKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"prepay_id":"wx-retry-ok"}`))
	}))
	defer server.Close()

	client := testClient(t, keyPEM, key)
	client.baseURL = server.URL
	transport := &flakyTransport{failures: 1, base: http.DefaultTransport}
	client.httpClient = &http.Client{Transport: transport, Timeout: 10 * time.Second}

	prepayID, err := client.Prepay(context.Background(), PrepayRequest{OutTradeNo: "retry-1", AmountFen: 1})
	if err != nil {
		t.Fatalf("Prepay with one network failure should retry: %v", err)
	}
	if prepayID != "wx-retry-ok" || transport.calls != 2 {
		t.Fatalf("prepay_id = %q, calls = %d, want retry once", prepayID, transport.calls)
	}
}

func TestPrepay_NetworkExhausted(t *testing.T) {
	keyPEM, key := testKey(t)
	client := testClient(t, keyPEM, key)
	// 指向已关闭的地址：传输层错误，重试后仍失败。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client.baseURL = server.URL
	server.Close()

	_, err := client.Prepay(context.Background(), PrepayRequest{OutTradeNo: "down-1", AmountFen: 1})
	var wxErr *Error
	if errors.As(err, &wxErr) {
		t.Fatalf("network error should not be *Error: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "wxpay prepay") {
		t.Fatalf("err = %v, want wrapped transport failure", err)
	}
}

func TestParsePrivateKey(t *testing.T) {
	keyPEM, _ := testKey(t)
	// 字面量 \n 形式（.env 单行值常见写法）。
	literal := strings.ReplaceAll(keyPEM, "\n", `\n`)
	if _, err := parsePrivateKey(literal); err != nil {
		t.Fatalf("parse literal \\n key: %v", err)
	}
	if _, err := parsePrivateKey("not-a-pem"); err == nil {
		t.Fatal("expected error for garbage input")
	}
}

func TestMockGateway(t *testing.T) {
	gw := NewMockGateway()
	prepayID, err := gw.Prepay(context.Background(), PrepayRequest{OutTradeNo: "mock-1", AmountFen: 98000})
	if err != nil {
		t.Fatalf("mock Prepay: %v", err)
	}
	if !strings.HasPrefix(prepayID, "mock-prepay-") {
		t.Fatalf("prepay_id = %q", prepayID)
	}
	params, err := gw.SignPayParams(prepayID)
	if err != nil {
		t.Fatalf("mock SignPayParams: %v", err)
	}
	if params.Package != "prepay_id="+prepayID || params.SignType != "RSA" || params.PaySign != "MOCK-SIGN" ||
		params.TimeStamp == "" || params.NonceStr == "" {
		t.Fatalf("mock params = %+v", params)
	}
}

// parseAuthParams 解析 Authorization 头的 key="value" 参数段。
func parseAuthParams(t *testing.T, header string) map[string]string {
	t.Helper()
	idx := strings.Index(header, " ")
	if idx < 0 {
		t.Fatalf("bad authorization header: %q", header)
	}
	out := map[string]string{}
	for _, part := range strings.Split(header[idx+1:], ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			t.Fatalf("bad auth part %q in %q", part, header)
		}
		out[kv[0]] = strings.Trim(kv[1], `"`)
	}
	return out
}
