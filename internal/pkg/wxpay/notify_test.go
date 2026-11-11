package wxpay

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// encryptNotify 用 APIv3 密钥加密交易明文，返回 base64 密文与 nonce。
func encryptNotify(t *testing.T, plaintext []byte, associatedData string) (string, string) {
	t.Helper()
	block, err := aes.NewCipher([]byte(testAPIv3Key))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new gcm: %v", err)
	}
	nonce := []byte("0123456789ab")
	ciphertext := gcm.Seal(nil, nonce, plaintext, []byte(associatedData))
	return base64.StdEncoding.EncodeToString(ciphertext), string(nonce)
}

// signedNotify 构造一份带合法签名的微信支付回调报文与请求头。
func signedNotify(t *testing.T, client *Client, key *rsa.PrivateKey, transaction string) ([]byte, NotifyHeaders) {
	t.Helper()
	ciphertext, nonce := encryptNotify(t, []byte(transaction), "transaction")
	envelope := map[string]any{
		"id":         "ev-20270101000000",
		"event_type": "TRANSACTION.SUCCESS",
		"resource": map[string]any{
			"algorithm":       "AEAD_AES_256_GCM",
			"ciphertext":      ciphertext,
			"associated_data": "transaction",
			"nonce":           nonce,
			"original_type":   "transaction",
		},
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return body, signHeaders(t, client, key, body)
}

// signHeaders 对回调报文签名（时间戳取 client.now）。
func signHeaders(t *testing.T, client *Client, key *rsa.PrivateKey, body []byte) NotifyHeaders {
	t.Helper()
	timestamp := strconv.FormatInt(client.now().Unix(), 10)
	nonce := "notify-nonce-1"
	digest := sha256.Sum256([]byte(timestamp + "\n" + nonce + "\n" + string(body) + "\n"))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign notify: %v", err)
	}
	return NotifyHeaders{
		Timestamp: timestamp,
		Nonce:     nonce,
		Signature: base64.StdEncoding.EncodeToString(sig),
		Serial:    client.platformSerial,
	}
}

const testTransactionJSON = `{
	"appid":"wx-appid","mchid":"1900000109",
	"out_trade_no":"2027010109300012345678",
	"transaction_id":"4200001234202701011234567890",
	"trade_state":"SUCCESS",
	"success_time":"2027-01-01T10:00:00+08:00",
	"amount":{"total":98000,"currency":"CNY"}
}`

func TestParseNotify_Success(t *testing.T) {
	keyPEM, key := testKey(t)
	client := testClient(t, keyPEM, key)
	body, headers := signedNotify(t, client, key, testTransactionJSON)

	tx, err := client.ParseNotify(headers, body)
	if err != nil {
		t.Fatalf("ParseNotify: %v", err)
	}
	if tx.OutTradeNo != "2027010109300012345678" || tx.TransactionID != "4200001234202701011234567890" ||
		tx.TradeState != TradeStateSuccess || tx.AmountTotal != 98000 ||
		tx.SuccessTime != "2027-01-01T10:00:00+08:00" {
		t.Fatalf("tx = %+v", tx)
	}
}

func TestParseNotify_Rejects(t *testing.T) {
	keyPEM, key := testKey(t)
	client := testClient(t, keyPEM, key)

	// 已签名的固定报文，供各失败分支复用。
	notifyBody, baseHeaders := signedNotify(t, client, key, testTransactionJSON)

	cases := []struct {
		name   string
		mutate func(body []byte, headers *NotifyHeaders) []byte
	}{
		{"签名被篡改", func(body []byte, _ *NotifyHeaders) []byte {
			return append(body[:len(body)-1], ' ')
		}},
		{"时间戳过期", func(_ []byte, h *NotifyHeaders) []byte {
			h.Timestamp = strconv.FormatInt(client.now().Add(-10*time.Minute).Unix(), 10)
			// 用过期时间戳重新签名，确保失败原因是时间戳而非签名。
			digest := sha256.Sum256([]byte(h.Timestamp + "\n" + h.Nonce + "\n" + string(notifyBody)))
			sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
			h.Signature = base64.StdEncoding.EncodeToString(sig)
			return notifyBody
		}},
		{"证书序列号不匹配", func(body []byte, h *NotifyHeaders) []byte {
			h.Serial = "DEADBEEF"
			return body
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			headers := baseHeaders
			body := tc.mutate(append([]byte(nil), notifyBody...), &headers)
			if _, err := client.ParseNotify(headers, body); !errors.Is(err, ErrInvalidNotify) {
				t.Fatalf("err = %v, want ErrInvalidNotify", err)
			}
		})
	}
}

func TestParseNotify_RejectsMismatchedMerchant(t *testing.T) {
	keyPEM, key := testKey(t)
	client := testClient(t, keyPEM, key)
	transaction := strings.Replace(testTransactionJSON, `"mchid":"1900000109"`, `"mchid":"1900000999"`, 1)
	body, headers := signedNotify(t, client, key, transaction)
	if _, err := client.ParseNotify(headers, body); !errors.Is(err, ErrInvalidNotify) {
		t.Fatalf("err = %v, want ErrInvalidNotify", err)
	}
}

func TestQueryOrder(t *testing.T) {
	keyPEM, key := testKey(t)

	var gotPath, gotQuery, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testTransactionJSON))
	}))
	defer server.Close()

	client := testClient(t, keyPEM, key)
	client.baseURL = server.URL
	tx, err := client.QueryOrder(context.Background(), "2027010109300012345678")
	if err != nil {
		t.Fatalf("QueryOrder: %v", err)
	}
	if tx.TradeState != TradeStateSuccess || tx.AmountTotal != 98000 {
		t.Fatalf("tx = %+v", tx)
	}
	if gotPath != "/v3/pay/transactions/out-trade-no/2027010109300012345678" ||
		gotQuery != "mchid=1900000109" {
		t.Fatalf("path = %q, query = %q", gotPath, gotQuery)
	}

	// 查单请求同样使用 V3 签名（canonical URL 含 query）。
	headers := parseAuthParams(t, gotAuth)
	signMessage := "GET\n" + gotPath + "?" + gotQuery + "\n" + headers["timestamp"] + "\n" +
		headers["nonce_str"] + "\n\n"
	digest := sha256.Sum256([]byte(signMessage))
	sig, err := base64.StdEncoding.DecodeString(headers["signature"])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("verify query signature: %v", err)
	}
}

func TestQueryOrder_WechatError(t *testing.T) {
	keyPEM, key := testKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"ORDER_NOT_EXIST","message":"订单不存在"}`))
	}))
	defer server.Close()

	client := testClient(t, keyPEM, key)
	client.baseURL = server.URL
	_, err := client.QueryOrder(context.Background(), "missing")
	var wxErr *Error
	if !errors.As(err, &wxErr) || wxErr.Code != "ORDER_NOT_EXIST" {
		t.Fatalf("err = %v, want *Error ORDER_NOT_EXIST", err)
	}
}

// TestQueryOrder_NotPayWithoutTransactionID 未支付订单的查单响应没有
// transaction_id，必须能正常解析（仅成功交易强制要求该字段）。
func TestQueryOrder_NotPayWithoutTransactionID(t *testing.T) {
	keyPEM, key := testKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"appid":"wx-appid","mchid":"1900000109",
			"out_trade_no":"2027010109300012345678",
			"trade_state":"NOTPAY",
			"amount":{"total":98000,"currency":"CNY"}
		}`))
	}))
	defer server.Close()

	client := testClient(t, keyPEM, key)
	client.baseURL = server.URL
	tx, err := client.QueryOrder(context.Background(), "2027010109300012345678")
	if err != nil {
		t.Fatalf("QueryOrder: %v", err)
	}
	if tx.TradeState != TradeStateNotPay || tx.TransactionID != "" {
		t.Fatalf("tx = %+v", tx)
	}
}

func TestMockGateway_NotifyAndQuery(t *testing.T) {
	gw := NewMockGateway()

	tx, err := gw.ParseNotify(NotifyHeaders{}, []byte(testTransactionJSON))
	if err != nil {
		t.Fatalf("mock ParseNotify: %v", err)
	}
	if tx.TransactionID != "4200001234202701011234567890" || tx.AmountTotal != 98000 {
		t.Fatalf("mock tx = %+v", tx)
	}
	if _, err := gw.ParseNotify(NotifyHeaders{}, []byte(`{"foo":1}`)); !errors.Is(err, ErrInvalidNotify) {
		t.Fatalf("malformed mock notify err = %v, want ErrInvalidNotify", err)
	}

	queried, err := gw.QueryOrder(context.Background(), "mock-order-1")
	if err != nil {
		t.Fatalf("mock QueryOrder: %v", err)
	}
	if queried.TradeState != TradeStateNotPay || queried.OutTradeNo != "mock-order-1" {
		t.Fatalf("mock query = %+v", queried)
	}
}
