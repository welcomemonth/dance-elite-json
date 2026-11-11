package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/meetmonth/dance-elite/internal/store"
)

// mustOrderNo 读取订单的商户订单号。
func mustOrderNo(t *testing.T, db *store.DB, orderID int64) string {
	t.Helper()
	var orderNo string
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT order_no FROM orders WHERE id = ?`, orderID).Scan(&orderNo); err != nil {
		t.Fatalf("query order_no: %v", err)
	}
	return orderNo
}

// mockNotifyBody 构造 mock 通道的明文回调报文（PAY_MOCK 下不加密不验签）。
func mockNotifyBody(orderNo, transactionID string, total int64) string {
	return fmt.Sprintf(`{
		"out_trade_no": %q,
		"transaction_id": %q,
		"trade_state": "SUCCESS",
		"success_time": "2027-01-01T10:00:00+08:00",
		"amount": {"total": %d, "currency": "CNY"}
	}`, orderNo, transactionID, total)
}

// setupOrderForNotify 创建赛季/赛事/用户/待支付订单，返回用户与订单 ID。
func TestWechatNotifyViaHTTP(t *testing.T) {
	engine, signer, db := newOrderTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID := mustCreatePublishedEvent(t, engine, auth, seasonID, "支付回调赛事")

	userID := mustInsertRouterUser(t, db, "o-notify-ok")
	orderID := mustSubmitAndGetOrder(t, engine, signer, db, eventID, userID, "key-notify-ok-1")
	orderNo := mustOrderNo(t, db, orderID)

	body := mockNotifyBody(orderNo, "wx-tx-router-1", 98000)
	w := postJSON(t, engine, http.MethodPost, "/api/v1/payments/wechat/notify", body, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("notify status = %d, body = %s", w.Code, w.Body)
	}
	var resp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode notify response: %v", err)
	}
	if resp.Code != "SUCCESS" {
		t.Fatalf("notify response = %+v", resp)
	}

	// 重复回调幂等：仍返回 SUCCESS，流水只有一条。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/payments/wechat/notify", body, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("duplicate notify status = %d, body = %s", w.Code, w.Body)
	}

	var orderStatus, regStatus string
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT status FROM orders WHERE id = ?`, orderID).Scan(&orderStatus); err != nil {
		t.Fatalf("query order status: %v", err)
	}
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT r.status FROM registrations r JOIN orders o ON o.registration_id = r.id WHERE o.id = ?`,
		orderID).Scan(&regStatus); err != nil {
		t.Fatalf("query registration status: %v", err)
	}
	if orderStatus != "paid" || regStatus != "active" {
		t.Fatalf("order/registration status = %s/%s, want paid/active", orderStatus, regStatus)
	}
	var payments int
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM payments WHERE order_id = ?`, orderID).Scan(&payments); err != nil {
		t.Fatalf("count payments: %v", err)
	}
	if payments != 1 {
		t.Fatalf("payments = %d, want 1", payments)
	}
}

func TestWechatNotifyRejectsBadRequests(t *testing.T) {
	engine, signer, db := newOrderTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID := mustCreatePublishedEvent(t, engine, auth, seasonID, "支付回调异常赛事")

	// 报文缺交易字段 → 验签/解析失败 → 401（微信重推）。
	w := postJSON(t, engine, http.MethodPost, "/api/v1/payments/wechat/notify", `{"foo":1}`, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid notify status = %d, want 401", w.Code)
	}

	// 金额不一致 → 500 + FAIL，订单保持待支付。
	userID := mustInsertRouterUser(t, db, "o-notify-amt")
	orderID := mustSubmitAndGetOrder(t, engine, signer, db, eventID, userID, "key-notify-amt1")
	orderNo := mustOrderNo(t, db, orderID)
	w = postJSON(t, engine, http.MethodPost, "/api/v1/payments/wechat/notify",
		mockNotifyBody(orderNo, "wx-tx-router-amt", 1), nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("amount mismatch status = %d, want 500", w.Code)
	}
	var resp struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Code != "FAIL" {
		t.Fatalf("amount mismatch response = %s", w.Body)
	}
	var status string
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT status FROM orders WHERE id = ?`, orderID).Scan(&status); err != nil {
		t.Fatalf("query order status: %v", err)
	}
	if status != "pending_payment" {
		t.Fatalf("order status = %q, want pending_payment", status)
	}
}

func TestOrderStatusViaHTTP(t *testing.T) {
	engine, signer, db := newOrderTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID := mustCreatePublishedEvent(t, engine, auth, seasonID, "订单状态赛事")

	userID := mustInsertRouterUser(t, db, "o-status-owner")
	orderID := mustSubmitAndGetOrder(t, engine, signer, db, eventID, userID, "key-status-http")

	// 未登录 → 40101。
	w := getJSON(t, engine, fmt.Sprintf("/api/v1/orders/%d", orderID), nil)
	assertCode(t, w, 40101)

	// 他人订单 → 40401。
	other := mustInsertRouterUser(t, db, "o-status-other")
	otherHeaders := map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, other)}
	w = getJSON(t, engine, fmt.Sprintf("/api/v1/orders/%d", orderID), otherHeaders)
	assertCode(t, w, 40401)

	// 非数字 ID → 40001。
	ownerHeaders := map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, userID)}
	w = getJSON(t, engine, "/api/v1/orders/abc", ownerHeaders)
	assertCode(t, w, 40001)

	// 本人订单：统一包装 + snake_case + 待支付（mock 查单恒 NOTPAY）。
	w = getJSON(t, engine, fmt.Sprintf("/api/v1/orders/%d", orderID), ownerHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("status query = %d, body = %s", w.Code, w.Body)
	}
	var raw struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if raw.Code != 0 {
		t.Fatalf("code = %d, body = %s", raw.Code, w.Body)
	}
	assertSnakeCaseKeys(t, "order status", raw.Data)
	assertObjectKeys(t, "order status", raw.Data,
		"order_id", "order_no", "status", "amount", "paid_at", "registration_id")
	if raw.Data["status"] != "pending_payment" {
		t.Fatalf("status = %v, want pending_payment", raw.Data["status"])
	}
	if raw.Data["paid_at"] != nil {
		t.Fatalf("paid_at = %v, want null", raw.Data["paid_at"])
	}
}
