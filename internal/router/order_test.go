package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/pkg/wxpay"
	"github.com/meetmonth/dance-elite/internal/service"
	"github.com/meetmonth/dance-elite/internal/store"
)

// routerFakeGateway 是 router 测试用的支付网关替身（幂等返回固定值）。
type routerFakeGateway struct{ calls int }

func (f *routerFakeGateway) Prepay(_ context.Context, _ wxpay.PrepayRequest) (string, error) {
	f.calls++
	return "fake-prepay-router", nil
}

func (f *routerFakeGateway) SignPayParams(prepayID string) (wxpay.PayParams, error) {
	return wxpay.PayParams{
		TimeStamp: "1767221400", NonceStr: "fake-nonce",
		Package: "prepay_id=" + prepayID, SignType: "RSA", PaySign: "FAKE-SIGN",
	}, nil
}

func newOrderTestEngine(t *testing.T) (*gin.Engine, *token.Signer, *store.DB) {
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
	mockPay := wxpay.NewMockGateway()
	return New(Deps{
		Stores:        stores,
		AdminAuth:     service.NewAdminAuthService(stores, signer),
		Seasons:       service.NewSeasonService(stores),
		Events:        service.NewEventService(stores),
		PublicEvents:  service.NewPublicEventService(stores),
		Registrations: service.NewRegistrationService(stores, players),
		Orders:        service.NewOrderService(stores, &routerFakeGateway{}),
		Payments:      service.NewPaymentService(stores, mockPay, mockPay),
		Tokens:        signer,
	}), signer, db
}

// mustSubmitAndGetOrder 提交报名并返回订单 ID（订单与报名单 1:1 但 ID 独立，从库中读）。
func mustSubmitAndGetOrder(t *testing.T, engine *gin.Engine, signer *token.Signer, db *store.DB, eventID, userID int64, key string) int64 {
	t.Helper()
	headers := map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, userID)}
	w := postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/events/%d/registrations", eventID),
		submitBody(routerValidIDCard, "classical", key), headers)
	if w.Code != http.StatusOK {
		t.Fatalf("submit status = %d, body = %s", w.Code, w.Body)
	}
	var submitted struct {
		Data struct {
			RegistrationID int64 `json:"registration_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &submitted); err != nil {
		t.Fatalf("decode submit: %v", err)
	}
	var orderID int64
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT id FROM orders WHERE registration_id = ?`, submitted.Data.RegistrationID).Scan(&orderID); err != nil {
		t.Fatalf("query order: %v", err)
	}
	return orderID
}

// TestPrepayViaHTTP 走完整链路：提交报名 → 预下单 → 断言 pay_params 结构。
func TestPrepayViaHTTP(t *testing.T) {
	engine, signer, db := newOrderTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID := mustCreatePublishedEvent(t, engine, auth, seasonID, "预下单赛事")
	userID := mustInsertRouterUser(t, db, "o-router-prepay")
	headers := map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, userID)}

	// 未登录 → 40101。
	w := postJSON(t, engine, http.MethodPost, "/api/v1/orders/1/prepay", "", nil)
	assertCode(t, w, 40101)

	orderID := mustSubmitAndGetOrder(t, engine, signer, db, eventID, userID, "key-prepay-http-1")

	w = postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/orders/%d/prepay", orderID), "", headers)
	if w.Code != http.StatusOK {
		t.Fatalf("prepay status = %d, body = %s", w.Code, w.Body)
	}
	var raw struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode prepay: %v", err)
	}
	if raw.Code != 0 {
		t.Fatalf("code = %d, body = %s", raw.Code, w.Body)
	}
	// pay_params 是微信协议 camelCase（文档级例外），外层字段保持 snake_case。
	assertSnakeCaseKeys(t, "prepay", raw.Data)
	assertObjectKeys(t, "prepay", raw.Data, "order_no", "pay_params")
	payParams, ok := raw.Data["pay_params"].(map[string]any)
	if !ok {
		t.Fatalf("pay_params = %v", raw.Data["pay_params"])
	}
	for k, v := range map[string]string{
		"timeStamp": "1767221400", "nonceStr": "fake-nonce",
		"package": "prepay_id=fake-prepay-router", "signType": "RSA", "paySign": "FAKE-SIGN",
	} {
		if payParams[k] != v {
			t.Fatalf("pay_params[%s] = %v, want %q", k, payParams[k], v)
		}
	}
}

func TestPrepayErrorsViaHTTP(t *testing.T) {
	engine, signer, db := newOrderTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID := mustCreatePublishedEvent(t, engine, auth, seasonID, "预下单错误赛事")

	// 每个请求换用户：预下单限流 10 次/分钟。
	headersOf := func(n int) map[string]string {
		userID := mustInsertRouterUser(t, db, fmt.Sprintf("o-prepay-err-%d", n))
		return map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, userID)}
	}

	// 非数字订单 ID → 40001。
	w := postJSON(t, engine, http.MethodPost, "/api/v1/orders/abc/prepay", "", headersOf(1))
	assertCode(t, w, 40001)

	// 不存在 → 40401。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/orders/9999/prepay", "", headersOf(2))
	assertCode(t, w, 40401)

	// 构造一个已支付订单：他人访问 → 40401，本人访问 → 40901。
	ownerID := mustInsertRouterUser(t, db, "o-prepay-err-owner")
	orderID := mustSubmitAndGetOrder(t, engine, signer, db, eventID, ownerID, "key-prepay-err-1")
	if _, err := db.SQL().ExecContext(context.Background(),
		`UPDATE orders SET status = 'paid' WHERE id = ?`, orderID); err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	w = postJSON(t, engine, http.MethodPost, fmt.Sprintf("/api/v1/orders/%d/prepay", orderID), "", headersOf(3))
	assertCode(t, w, 40401)
	ownerHeaders := map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, ownerID)}
	w = postJSON(t, engine, http.MethodPost, fmt.Sprintf("/api/v1/orders/%d/prepay", orderID), "", ownerHeaders)
	assertCode(t, w, 40901)
}

// TestPrepayRateLimit 预下单限流 10 次/分钟：第 11 次 → 42901。
func TestPrepayRateLimit(t *testing.T) {
	engine, signer, db := newOrderTestEngine(t)
	auth := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	seasonID := setupSeason(t, engine, auth)
	eventID := mustCreatePublishedEvent(t, engine, auth, seasonID, "预下单限流赛事")
	userID := mustInsertRouterUser(t, db, "o-prepay-limit")
	orderID := mustSubmitAndGetOrder(t, engine, signer, db, eventID, userID, "key-prepay-limit1")
	headers := map[string]string{"Authorization": "Bearer " + userTokenFor(t, signer, userID)}

	var last *httptest.ResponseRecorder
	for i := 0; i < 11; i++ {
		last = postJSON(t, engine, http.MethodPost,
			fmt.Sprintf("/api/v1/orders/%d/prepay", orderID), "", headers)
	}
	assertCode(t, last, 42901)
}
