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
	return New(Deps{
		Stores:        stores,
		AdminAuth:     service.NewAdminAuthService(stores, signer),
		Seasons:       service.NewSeasonService(stores),
		Events:        service.NewEventService(stores),
		PublicEvents:  service.NewPublicEventService(stores),
		Registrations: service.NewRegistrationService(stores, players),
		Waitlists:     service.NewWaitlistService(stores, players),
		Refunds:       service.NewRefundService(stores),
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
