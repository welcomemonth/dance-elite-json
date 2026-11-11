package wxpay

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRefund_SuccessAndSignature(t *testing.T) {
	keyPEM, key := testKey(t)

	var gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != refundPath {
			t.Errorf("path = %s, want %s", r.URL.Path, refundPath)
		}
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"refund_id":"50000001","out_refund_no":"TK202701","out_trade_no":"ORD-1","status":"SUCCESS"}`))
	}))
	defer server.Close()

	client := testClient(t, keyPEM, key)
	client.baseURL = server.URL
	client.refundNotifyURL = "https://example.com/refund/notify"
	result, err := client.Refund(context.Background(), RefundRequest{
		OutTradeNo:  "ORD-1",
		OutRefundNo: "TK202701",
		Reason:      "伤病退赛",
		AmountFen:   98000,
		TotalFen:    98000,
	})
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if result.RefundID != "50000001" || result.OutRefundNo != "TK202701" || result.Status != RefundStatusSuccess {
		t.Fatalf("result = %+v", result)
	}

	for _, want := range []string{`"out_trade_no":"ORD-1"`, `"out_refund_no":"TK202701"`,
		`"refund":98000`, `"total":98000`, `"notify_url":"https://example.com/refund/notify"`, `"reason":"伤病退赛"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("body %s should contain %s", gotBody, want)
		}
	}

	// 请求签名可按 V3 规则用公钥验签。
	headers := parseAuthParams(t, gotAuth)
	signMessage := "POST\n" + refundPath + "\n" + headers["timestamp"] + "\n" + headers["nonce_str"] + "\n" + gotBody + "\n"
	digest := sha256.Sum256([]byte(signMessage))
	sig, err := base64.StdEncoding.DecodeString(headers["signature"])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("verify refund signature: %v", err)
	}
}

func TestRefund_RejectsInvalidAmount(t *testing.T) {
	keyPEM, key := testKey(t)
	client := testClient(t, keyPEM, key)

	for _, req := range []RefundRequest{
		{OutTradeNo: "ORD-1", OutRefundNo: "TK-1", AmountFen: 0, TotalFen: 100},
		{OutTradeNo: "ORD-1", OutRefundNo: "TK-1", AmountFen: 200, TotalFen: 100},
		{OutTradeNo: "", OutRefundNo: "TK-1", AmountFen: 100, TotalFen: 100},
	} {
		if _, err := client.Refund(context.Background(), req); err == nil {
			t.Fatalf("expected error for %+v", req)
		}
	}
}

func TestRefund_WechatError(t *testing.T) {
	keyPEM, key := testKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"INVALID_REQUEST","message":"退款金额非法"}`))
	}))
	defer server.Close()

	client := testClient(t, keyPEM, key)
	client.baseURL = server.URL
	_, err := client.Refund(context.Background(), RefundRequest{
		OutTradeNo: "ORD-1", OutRefundNo: "TK-1", AmountFen: 1, TotalFen: 1,
	})
	var wxErr *Error
	if !errors.As(err, &wxErr) || wxErr.Code != "INVALID_REQUEST" {
		t.Fatalf("err = %v, want *Error INVALID_REQUEST", err)
	}
}

const testRefundNotifyJSON = `{
	"mchid":"1900000109",
	"out_trade_no":"2027010109300012345678",
	"transaction_id":"4200001234202701011234567890",
	"out_refund_no":"TK202701",
	"refund_id":"50000001",
	"refund_status":"SUCCESS",
	"success_time":"2027-01-01T11:00:00+08:00",
	"amount":{"total":98000,"refund":98000,"payer_total":98000,"payer_refund":98000}
}`

func TestParseRefundNotify_Success(t *testing.T) {
	keyPEM, key := testKey(t)
	client := testClient(t, keyPEM, key)
	body, headers := signedNotify(t, client, key, testRefundNotifyJSON)

	result, err := client.ParseRefundNotify(headers, body)
	if err != nil {
		t.Fatalf("ParseRefundNotify: %v", err)
	}
	if result.RefundID != "50000001" || result.OutRefundNo != "TK202701" ||
		result.RefundStatus != RefundStatusSuccess || result.RefundAmount != 98000 || result.TotalAmount != 98000 {
		t.Fatalf("result = %+v", result)
	}
}

func TestParseRefundNotify_Invalid(t *testing.T) {
	keyPEM, key := testKey(t)
	client := testClient(t, keyPEM, key)

	// 验签失败：篡改报文。
	body, headers := signedNotify(t, client, key, testRefundNotifyJSON)
	if _, err := client.ParseRefundNotify(headers, append(body, ' ')); !errors.Is(err, ErrInvalidNotify) {
		t.Fatalf("tampered body err = %v, want ErrInvalidNotify", err)
	}

	// 缺少 out_refund_no。
	body, headers = signedNotify(t, client, key, `{"refund_status":"SUCCESS","refund_id":"1"}`)
	if _, err := client.ParseRefundNotify(headers, body); !errors.Is(err, ErrInvalidNotify) {
		t.Fatalf("missing out_refund_no err = %v, want ErrInvalidNotify", err)
	}
}

func TestMockGateway_RefundAndNotify(t *testing.T) {
	gw := NewMockGateway()
	result, err := gw.Refund(context.Background(), RefundRequest{
		OutTradeNo: "ORD-1", OutRefundNo: "TK-1", AmountFen: 100, TotalFen: 100,
	})
	if err != nil {
		t.Fatalf("mock Refund: %v", err)
	}
	if !strings.HasPrefix(result.RefundID, "mock-refund-") || result.Status != RefundStatusSuccess {
		t.Fatalf("mock refund = %+v", result)
	}

	notified, err := gw.ParseRefundNotify(NotifyHeaders{}, []byte(testRefundNotifyJSON))
	if err != nil {
		t.Fatalf("mock ParseRefundNotify: %v", err)
	}
	if notified.RefundID != "50000001" {
		t.Fatalf("mock notify = %+v", notified)
	}
	if _, err := gw.ParseRefundNotify(NotifyHeaders{}, []byte(`{"foo":1}`)); !errors.Is(err, ErrInvalidNotify) {
		t.Fatalf("malformed mock notify err = %v, want ErrInvalidNotify", err)
	}
}
