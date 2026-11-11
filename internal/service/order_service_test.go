package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/wxpay"
)

// fakeGateway 是可注入行为的 wxpay.Gateway 测试替身。
type fakeGateway struct {
	prepayID string
	err      error
	calls    int
}

func (f *fakeGateway) Prepay(_ context.Context, _ wxpay.PrepayRequest) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	if f.prepayID == "" {
		return "fake-prepay-1", nil
	}
	return f.prepayID, nil
}

func (f *fakeGateway) SignPayParams(prepayID string) (wxpay.PayParams, error) {
	return wxpay.PayParams{
		TimeStamp: "1767221400",
		NonceStr:  "fake-nonce",
		Package:   "prepay_id=" + prepayID,
		SignType:  "RSA",
		PaySign:   "FAKE-SIGN",
	}, nil
}

// orderFixture 复用 T9 夹具并追加 OrderService + fake 网关。
type orderFixture struct {
	*regFixture
	gateway *fakeGateway
	svc     *OrderService
}

func newOrderFixture(t *testing.T) *orderFixture {
	t.Helper()
	f := newRegFixture(t)
	gateway := &fakeGateway{}
	return &orderFixture{regFixture: f, gateway: gateway, svc: NewOrderService(f.stores, gateway)}
}

// mustSubmitOrder 走 T9 提交报名产生一个待支付订单，返回订单 ID。
func mustSubmitOrder(t *testing.T, f *orderFixture, userID int64, key string) int64 {
	t.Helper()
	eventID := mustCreateRegisteringEvent(t, f.regFixture, "支付赛事-"+key, defaultRegGroups)
	if _, err := f.regSvc.Submit(context.Background(), userID, eventID,
		submitReq(validIDCardMale2016, model.DanceTypeClassical, key)); err != nil {
		t.Fatalf("submit: %v", err)
	}
	order, err := f.stores.Orders.GetByRequest(context.Background(), userID, key)
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	if order == nil {
		t.Fatalf("order by request %q not found", key)
	}
	return order.ID
}

func TestPrepay_Success(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-prepay-ok")
	orderID := mustSubmitOrder(t, f, userID, "key-prepay-ok-01")

	resp, err := f.svc.Prepay(ctx, userID, "o-openid", orderID)
	if err != nil {
		t.Fatalf("Prepay: %v", err)
	}
	if resp.OrderNo == "" {
		t.Fatalf("resp = %+v", resp)
	}
	want := model.WxPayParams{
		TimeStamp: "1767221400", NonceStr: "fake-nonce",
		Package: "prepay_id=fake-prepay-1", SignType: "RSA", PaySign: "FAKE-SIGN",
	}
	if resp.PayParams != want {
		t.Fatalf("pay_params = %+v, want %+v", resp.PayParams, want)
	}
	// prepay_id 已持久化，网关只被调用一次。
	if f.gateway.calls != 1 {
		t.Fatalf("gateway calls = %d, want 1", f.gateway.calls)
	}
	order, err := f.stores.Orders.Get(ctx, orderID)
	if err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if order.PrepayID == nil || *order.PrepayID != "fake-prepay-1" {
		t.Fatalf("prepay_id = %v", order.PrepayID)
	}
}

func TestPrepay_IdempotentReuse(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-prepay-idem")
	orderID := mustSubmitOrder(t, f, userID, "key-prepay-idem1")

	first, err := f.svc.Prepay(ctx, userID, "o-openid", orderID)
	if err != nil {
		t.Fatalf("first Prepay: %v", err)
	}
	second, err := f.svc.Prepay(ctx, userID, "o-openid", orderID)
	if err != nil {
		t.Fatalf("second Prepay: %v", err)
	}
	if first.PayParams.Package != second.PayParams.Package {
		t.Fatalf("prepay package differ: %q vs %q", first.PayParams.Package, second.PayParams.Package)
	}
	if f.gateway.calls != 1 {
		t.Fatalf("gateway calls = %d, want 1（复用 prepay_id 不再下单）", f.gateway.calls)
	}
}

func TestPrepay_OrderNotFoundOrNotOwned(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-prepay-own")
	orderID := mustSubmitOrder(t, f, userID, "key-prepay-own1")

	if _, err := f.svc.Prepay(ctx, userID, "o-openid", 9999); err == nil || bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("missing order err = %v, want 40401", err)
	}
	otherUser := mustInsertRegUser(t, f.db, "o-prepay-other")
	if _, err := f.svc.Prepay(ctx, otherUser, "o-openid", orderID); err == nil || bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("not owned err = %v, want 40401", err)
	}
	if f.gateway.calls != 0 {
		t.Fatalf("gateway calls = %d, want 0（校验失败不调网关）", f.gateway.calls)
	}
}

func TestPrepay_OrderStatusConflict(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-prepay-st")
	orderID := mustSubmitOrder(t, f, userID, "key-prepay-st-1")

	if _, err := f.db.SQL().ExecContext(ctx,
		`UPDATE orders SET status = 'paid', paid_at = '2027-01-01T10:00:00+08:00' WHERE id = ?`, orderID); err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	_, err := f.svc.Prepay(ctx, userID, "o-openid", orderID)
	if err == nil || bizCode(t, err) != httpx.CodeConflict {
		t.Fatalf("paid order err = %v, want 40901", err)
	}
	if f.gateway.calls != 0 {
		t.Fatalf("gateway calls = %d, want 0", f.gateway.calls)
	}
}

func TestPrepay_GatewayFailures(t *testing.T) {
	ctx := context.Background()

	// 微信业务错误（*wxpay.Error）→ 50001，用户文案固定。
	f := newOrderFixture(t)
	f.gateway.err = &wxpay.Error{Status: 400, Code: "OUT_TRADE_NO_USED", Message: "订单号已存在"}
	userID := mustInsertRegUser(t, f.db, "o-prepay-wxerr")
	orderID := mustSubmitOrder(t, f, userID, "key-prepay-wxerr")
	_, err := f.svc.Prepay(ctx, userID, "o-openid", orderID)
	biz := bizErr(t, err)
	if biz.Code != httpx.CodeInternal || biz.Message != "拉起支付失败，请稍后重试" {
		t.Fatalf("biz = %+v, want 50001 拉起支付失败", biz)
	}
	if biz.Cause == nil {
		t.Fatal("cause should carry wechat error for logging")
	}

	// 网络错误（普通 error）→ 50001，且不持久化 prepay_id。
	f2 := newOrderFixture(t)
	f2.gateway.err = errors.New("network down")
	userID2 := mustInsertRegUser(t, f2.db, "o-prepay-net")
	orderID2 := mustSubmitOrder(t, f2, userID2, "key-prepay-net-1")
	if _, err := f2.svc.Prepay(ctx, userID2, "o-openid", orderID2); err == nil || bizCode(t, err) != httpx.CodeInternal {
		t.Fatalf("network err = %v, want 50001", err)
	}
	order2, err := f2.stores.Orders.Get(ctx, orderID2)
	if err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if order2.PrepayID != nil {
		t.Fatalf("prepay_id = %v, want nil（网关失败不写入）", *order2.PrepayID)
	}
}

// TestPrepay_ConcurrentSameOrder 并发预下单同一订单：按订单串行化，
// 网关只应被调用一次，两个请求拿到同一 prepay_id。
func TestPrepay_ConcurrentSameOrder(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-prepay-conc")
	orderID := mustSubmitOrder(t, f, userID, "key-prepay-conc1")

	type result struct {
		pkg string
		err error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := f.svc.Prepay(ctx, userID, "o-openid", orderID)
			if err != nil {
				results <- result{err: err}
				return
			}
			results <- result{pkg: resp.PayParams.Package}
		}()
	}
	wg.Wait()
	close(results)

	pkgs := make([]string, 0, 2)
	for r := range results {
		if r.err != nil {
			t.Fatalf("concurrent prepay failed: %v", r.err)
		}
		pkgs = append(pkgs, r.pkg)
	}
	if pkgs[0] != pkgs[1] {
		t.Fatalf("concurrent prepay packages differ: %v", pkgs)
	}
	if f.gateway.calls != 1 {
		t.Fatalf("gateway calls = %d, want 1（按订单串行化，不向微信重复下单）", f.gateway.calls)
	}
}
