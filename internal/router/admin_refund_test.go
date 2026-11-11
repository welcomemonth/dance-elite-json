package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/pkg/wxpay"
	"github.com/meetmonth/dance-elite/internal/service"
	"github.com/meetmonth/dance-elite/internal/store"
)

// newRefundTestEngine 装配含退款工单路由的测试引擎。
func newRefundTestEngine(t *testing.T) (*gin.Engine, *token.Signer, *store.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db.SQL()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Seed(context.Background(), store.SeedOptions{
		AdminUsername: "admin", AdminPassword: "Init@12345",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stores := store.New(db)
	signer := token.NewSigner("test-secret", time.Hour)
	players := service.NewPlayerService(stores, "test-hmac-key")
	payGateway := wxpay.NewMockGateway()
	return New(Deps{
		Stores:        stores,
		AdminAuth:     service.NewAdminAuthService(stores, signer),
		Seasons:       service.NewSeasonService(stores),
		Events:        service.NewEventService(stores),
		PublicEvents:  service.NewPublicEventService(stores),
		Registrations: service.NewRegistrationService(stores, players),
		Waitlists:     service.NewWaitlistService(stores, players),
		Refunds:       service.NewRefundService(stores, payGateway, payGateway),
		CSTickets:     service.NewCSTicketService(stores),
		Tokens:        signer,
	}), signer, db
}

// TestCreateRefundTicketHTTP 覆盖登记接口的鉴权、角色、响应字段与错误码。
func TestCreateRefundTicketHTTP(t *testing.T) {
	engine, signer, db := newRefundTestEngine(t)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	csHeaders := map[string]string{"Authorization": "Bearer " + csAdminToken(t, signer)}
	seasonID := setupSeason(t, engine, adminHeaders)
	eventID, groupID := mustCreateWaitlistEvent(t, engine, db, adminHeaders, seasonID, "退款接口赛事", 1)
	userID := mustInsertRouterUser(t, db, "o-refund-http")
	orderNo := mustRouterPaidOrder(t, db, eventID, groupID, userID, "Prefund1", orderNoFor("refund-http"))

	body := fmt.Sprintf(`{"order_no":%q,"reason":"孩子伤病需退赛"}`, orderNo)

	// 未登录 → 40101。
	assertCode(t, postJSON(t, engine, http.MethodPost, "/api/v1/admin/refund-tickets", body, nil), 40101)

	// cs 可代录 → 200；响应 snake_case 且不含内部字段。
	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/refund-tickets", body, csHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("create ticket status = %d, body = %s", w.Code, w.Body)
	}
	var raw struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if raw.Code != 0 {
		t.Fatalf("code = %d, body = %s", raw.Code, w.Body)
	}
	assertSnakeCaseKeys(t, "create refund ticket", raw.Data)
	assertObjectKeys(t, "create refund ticket", raw.Data, "ticket_id", "ticket_no", "status")
	if raw.Data["status"] != "pending" || raw.Data["ticket_no"] == "" {
		t.Fatalf("create data = %v", raw.Data)
	}

	// 重复登记 → 40901。
	assertCode(t, postJSON(t, engine, http.MethodPost, "/api/v1/admin/refund-tickets", body, csHeaders), 40901)

	// 参数缺失 → 40001；订单不存在 → 40401。
	assertCode(t, postJSON(t, engine, http.MethodPost, "/api/v1/admin/refund-tickets",
		`{"reason":"原因"}`, adminHeaders), 40001)
	assertCode(t, postJSON(t, engine, http.MethodPost, "/api/v1/admin/refund-tickets",
		`{"order_no":"ORD-missing","reason":"原因"}`, adminHeaders), 40401)

	// 报名截止后登记 → 41015。
	lateOrderNo := mustRouterPaidOrder(t, db, eventID, groupID, userID, "Prefund2", orderNoFor("refund-late"))
	if _, err := db.SQL().ExecContext(context.Background(),
		`UPDATE events SET reg_start_at = '2019-12-01T00:00:00+08:00',
		 reg_end_at = '2020-01-01T00:00:00+08:00' WHERE id = ?`, eventID); err != nil {
		t.Fatalf("update reg end: %v", err)
	}
	assertCode(t, postJSON(t, engine, http.MethodPost, "/api/v1/admin/refund-tickets",
		fmt.Sprintf(`{"order_no":%q,"reason":"原因"}`, lateOrderNo), adminHeaders), 41015)
}

// orderNoFor 生成固定前缀 + 纳秒后缀的订单号（测试内唯一）。
func orderNoFor(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// mustCreateRefundTicketHTTP 通过接口代录退款工单，返回 ticket_id 与 ticket_no。
func mustCreateRefundTicketHTTP(t *testing.T, engine *gin.Engine, headers map[string]string, orderNo string) (int64, string) {
	t.Helper()
	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/refund-tickets",
		fmt.Sprintf(`{"order_no":%q,"reason":"伤病退赛"}`, orderNo), headers)
	if w.Code != http.StatusOK {
		t.Fatalf("create ticket status = %d, body = %s", w.Code, w.Body)
	}
	var created struct {
		Data struct {
			TicketID int64  `json:"ticket_id"`
			TicketNo string `json:"ticket_no"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode ticket: %v", err)
	}
	return created.Data.TicketID, created.Data.TicketNo
}

// TestRefundApproveRejectHTTP 覆盖同意/驳回接口的角色、参数与状态机。
func TestRefundApproveRejectHTTP(t *testing.T) {
	engine, signer, db := newRefundTestEngine(t)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	csHeaders := map[string]string{"Authorization": "Bearer " + csAdminToken(t, signer)}
	seasonID := setupSeason(t, engine, adminHeaders)
	eventID, groupID := mustCreateWaitlistEvent(t, engine, db, adminHeaders, seasonID, "退款处理赛事", 1)
	userID := mustInsertRouterUser(t, db, "o-refund-act")

	orderNo := mustRouterPaidOrder(t, db, eventID, groupID, userID, "Prefund3", orderNoFor("refund-act"))
	ticketID, _ := mustCreateRefundTicketHTTP(t, engine, adminHeaders, orderNo)

	// cs 无同意/驳回权限 → 40301。
	assertCode(t, postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/refund-tickets/%d/approve", ticketID), `{"amount":98000}`, csHeaders), 40301)
	assertCode(t, postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/refund-tickets/%d/reject", ticketID), `{"reject_reason":"原因"}`, csHeaders), 40301)

	// 参数缺失 → 40001。
	assertCode(t, postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/refund-tickets/%d/approve", ticketID), `{}`, adminHeaders), 40001)
	assertCode(t, postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/refund-tickets/%d/reject", ticketID), `{}`, adminHeaders), 40001)

	// 同意成功；响应字段收敛。
	w := postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/refund-tickets/%d/approve", ticketID), `{"amount":98000}`, adminHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("approve status = %d, body = %s", w.Code, w.Body)
	}
	var approved struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &approved); err != nil {
		t.Fatalf("decode approve: %v", err)
	}
	assertSnakeCaseKeys(t, "refund approve", approved.Data)
	assertObjectKeys(t, "refund approve", approved.Data, "ticket_no", "status", "refund_id", "order_status")
	if approved.Data["status"] != "approved" || approved.Data["order_status"] != "refunding" ||
		approved.Data["refund_id"] == "" {
		t.Fatalf("approve data = %v", approved.Data)
	}

	// 已同意 → 再次同意/驳回均 41013。
	assertCode(t, postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/refund-tickets/%d/approve", ticketID), `{"amount":98000}`, adminHeaders), 41013)
	assertCode(t, postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/refund-tickets/%d/reject", ticketID), `{"reject_reason":"原因"}`, adminHeaders), 41013)

	// 另一张工单允许驳回；订单保持 paid。
	rejectOrderNo := mustRouterPaidOrder(t, db, eventID, groupID, userID, "Prefund4", orderNoFor("refund-reject"))
	rejectTicketID, _ := mustCreateRefundTicketHTTP(t, engine, adminHeaders, rejectOrderNo)
	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/refund-tickets/%d/reject", rejectTicketID),
		`{"reject_reason":"已过退费期"}`, adminHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("reject status = %d, body = %s", w.Code, w.Body)
	}
	var rejected struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rejected); err != nil {
		t.Fatalf("decode reject: %v", err)
	}
	if rejected.Data["status"] != "rejected" || rejected.Data["order_status"] != "paid" {
		t.Fatalf("reject data = %v", rejected.Data)
	}
}

// TestRefundNotifyHTTP 覆盖退款回调：成功 200、幂等与验签失败。
func TestRefundNotifyHTTP(t *testing.T) {
	engine, _, db := newRefundTestEngine(t)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, adminHeaders)
	eventID, groupID := mustCreateWaitlistEvent(t, engine, db, adminHeaders, seasonID, "退款回调赛事", 1)
	userID := mustInsertRouterUser(t, db, "o-refund-notify")
	orderNo := mustRouterPaidOrder(t, db, eventID, groupID, userID, "Prefund5", orderNoFor("refund-notify"))
	if _, err := db.SQL().ExecContext(context.Background(),
		`UPDATE event_groups SET taken = 1 WHERE id = ?`, groupID); err != nil {
		t.Fatalf("take slot: %v", err)
	}
	ticketID, ticketNo := mustCreateRefundTicketHTTP(t, engine, adminHeaders, orderNo)

	w := postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/refund-tickets/%d/approve", ticketID), `{"amount":98000}`, adminHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("approve status = %d, body = %s", w.Code, w.Body)
	}
	var approved struct {
		Data struct {
			RefundID string `json:"refund_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &approved); err != nil {
		t.Fatalf("decode approve: %v", err)
	}

	body := fmt.Sprintf(`{"mchid":"1900000109","out_trade_no":%q,"out_refund_no":%q,
		"refund_id":%q,"refund_status":"SUCCESS","amount":{"total":98000,"refund":98000,"currency":"CNY"}}`,
		orderNo, ticketNo, approved.Data.RefundID)

	// 非法报文 → 401（验签/解析失败，微信会重推）。
	bad := postJSON(t, engine, http.MethodPost, "/api/v1/payments/wechat/refund-notify", `{"foo":1}`, nil)
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad notify status = %d, body = %s", bad.Code, bad.Body)
	}

	// 成功回调 → 微信协议响应；重复回调幂等。
	for i := 0; i < 2; i++ {
		w = postJSON(t, engine, http.MethodPost, "/api/v1/payments/wechat/refund-notify", body, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("refund notify #%d status = %d, body = %s", i, w.Code, w.Body)
		}
		var resp struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Code != "SUCCESS" {
			t.Fatalf("refund notify #%d resp = %s", i, w.Body)
		}
	}

	var orderStatus string
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT status FROM orders WHERE order_no = ?`, orderNo).Scan(&orderStatus); err != nil {
		t.Fatalf("query order: %v", err)
	}
	if orderStatus != "refunded" {
		t.Fatalf("order status = %s, want refunded", orderStatus)
	}
	var taken int
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT taken FROM event_groups WHERE id = ?`, groupID).Scan(&taken); err != nil {
		t.Fatalf("query group: %v", err)
	}
	if taken != 0 {
		t.Fatalf("taken = %d, want 0", taken)
	}
}
