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

// countOrderPayments 返回订单的支付流水条数（取消留档断言用）。
func countOrderPayments(t *testing.T, f *orderFixture, orderID int64) int {
	t.Helper()
	var n int
	if err := f.db.SQL().QueryRow(`SELECT COUNT(*) FROM payments WHERE order_id = ?`, orderID).Scan(&n); err != nil {
		t.Fatalf("count payments: %v", err)
	}
	return n
}

// registrationStatus 读取报名单状态。
func registrationStatus(t *testing.T, f *orderFixture, id int64) string {
	t.Helper()
	var status string
	if err := f.db.SQL().QueryRow(`SELECT status FROM registrations WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("query registration status: %v", err)
	}
	return status
}

// TestCancel_SuccessAndReregister 取消成功：订单/报名单置 canceled、名额释放、
// 写关闭流水留档；重复取消幂等；取消后可立即重新报名（T12 核心链路）。
func TestCancel_SuccessAndReregister(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-cancel-ok")
	eventID := mustCreateRegisteringEvent(t, f.regFixture, "取消赛事", defaultRegGroups)
	groupID := mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)

	submitted, err := f.regSvc.Submit(ctx, userID, eventID,
		submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-cancel-ok-01"))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if took := groupTaken(t, f.db, groupID); took != 1 {
		t.Fatalf("taken = %d, want 1", took)
	}

	resp, err := f.svc.Cancel(ctx, userID, submitted.OrderID, "")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if resp.Status != model.OrderStatusCanceled || resp.CancelReason == nil ||
		*resp.CancelReason != model.OrderCancelReasonUserCancel || resp.CanceledAt == nil {
		t.Fatalf("resp = %+v", resp)
	}
	order, err := f.stores.Orders.Get(ctx, submitted.OrderID)
	if err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if order.Status != model.OrderStatusCanceled || order.CanceledAt == nil ||
		order.CancelReason == nil || *order.CancelReason != model.OrderCancelReasonUserCancel {
		t.Fatalf("order = %+v", order)
	}
	if status := registrationStatus(t, f, submitted.RegistrationID); status != model.RegistrationStatusCanceled {
		t.Fatalf("registration status = %q, want canceled", status)
	}
	if took := groupTaken(t, f.db, groupID); took != 0 {
		t.Fatalf("taken = %d, want 0（名额已释放）", took)
	}
	var payType, payStatus string
	var amount int64
	if err := f.db.SQL().QueryRow(
		`SELECT type, status, amount FROM payments WHERE order_id = ?`, submitted.OrderID).
		Scan(&payType, &payStatus, &amount); err != nil {
		t.Fatalf("query cancel payment: %v", err)
	}
	if payType != model.PaymentTypePay || payStatus != model.PaymentStatusClosed || amount != submitted.Amount {
		t.Fatalf("payment = %s/%s/%d", payType, payStatus, amount)
	}

	// 重复取消：幂等，不重复释放名额、不重复写流水。
	again, err := f.svc.Cancel(ctx, userID, submitted.OrderID, model.OrderCancelReasonPayFailed)
	if err != nil {
		t.Fatalf("second Cancel: %v", err)
	}
	if again.Status != model.OrderStatusCanceled {
		t.Fatalf("second resp = %+v", again)
	}
	if took := groupTaken(t, f.db, groupID); took != 0 {
		t.Fatalf("taken = %d, want 0（重复取消不重复递减）", took)
	}
	if n := countOrderPayments(t, f, submitted.OrderID); n != 1 {
		t.Fatalf("payments = %d, want 1（重复取消不重复写流水）", n)
	}

	// 重新报名：取消后可直接走 T9 校验链，无需等待。
	resubmitted, err := f.regSvc.Submit(ctx, userID, eventID,
		submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-cancel-again1"))
	if err != nil {
		t.Fatalf("resubmit after cancel: %v", err)
	}
	if resubmitted.OrderID == submitted.OrderID {
		t.Fatal("resubmit should create a new order")
	}
	if status := registrationStatus(t, f, resubmitted.RegistrationID); status != model.RegistrationStatusPendingPayment {
		t.Fatalf("resubmitted registration status = %q", status)
	}
	if took := groupTaken(t, f.db, groupID); took != 1 {
		t.Fatalf("taken = %d, want 1（重新报名占回名额）", took)
	}
}

// TestCancel_RejectsPaidAndInvalid 已支付订单不可取消（40901）；
// 非本人/不存在 40401；非法取消原因 40001，且均无副作用。
func TestCancel_RejectsPaidAndInvalid(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-cancel-rej")
	eventID := mustCreateRegisteringEvent(t, f.regFixture, "取消拒绝赛事", defaultRegGroups)
	groupID := mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)

	submitted, err := f.regSvc.Submit(ctx, userID, eventID,
		submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-cancel-rej-1"))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := f.db.SQL().ExecContext(ctx,
		`UPDATE orders SET status = 'paid', paid_at = '2027-01-01T10:00:00+08:00' WHERE id = ?`,
		submitted.OrderID); err != nil {
		t.Fatalf("mark paid: %v", err)
	}

	if _, err := f.svc.Cancel(ctx, userID, submitted.OrderID, ""); err == nil || bizCode(t, err) != httpx.CodeConflict {
		t.Fatalf("paid order cancel err = %v, want 40901", err)
	}
	if status := registrationStatus(t, f, submitted.RegistrationID); status != model.RegistrationStatusPendingPayment {
		t.Fatalf("registration status = %q（已支付订单取消必须是 no-op）", status)
	}
	if took := groupTaken(t, f.db, groupID); took != 1 {
		t.Fatalf("taken = %d, want 1", took)
	}

	otherUser := mustInsertRegUser(t, f.db, "o-cancel-other")
	if _, err := f.svc.Cancel(ctx, otherUser, submitted.OrderID, ""); err == nil || bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("not owned err = %v, want 40401", err)
	}
	if _, err := f.svc.Cancel(ctx, userID, 9999, ""); err == nil || bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("missing order err = %v, want 40401", err)
	}
	if _, err := f.svc.Cancel(ctx, userID, submitted.OrderID, model.OrderCancelReasonSystem); err == nil || bizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("system reason err = %v, want 40001", err)
	}
}

// TestCancel_ReleasesSlotForConcurrentGrab 取消释放名额后，两个用户并发抢
// 最后一个名额只有一个成功（T9 并发测试 ④ 的 T12 回归）。
func TestCancel_ReleasesSlotForConcurrentGrab(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	groups := []model.AdminEventGroupInput{
		{Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Quota: 1},
	}
	eventID := mustCreateRegisteringEvent(t, f.regFixture, "取消并发赛事", groups)
	groupID := mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)
	userA := mustInsertRegUser(t, f.db, "o-cancel-conc-a")
	userB := mustInsertRegUser(t, f.db, "o-cancel-conc-b")

	first, err := f.regSvc.Submit(ctx, userA, eventID,
		submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-cancel-conc-a1"))
	if err != nil {
		t.Fatalf("submit A: %v", err)
	}
	if _, err := f.svc.Cancel(ctx, userA, first.OrderID, ""); err != nil {
		t.Fatalf("cancel A: %v", err)
	}
	if took := groupTaken(t, f.db, groupID); took != 0 {
		t.Fatalf("taken = %d, want 0", took)
	}

	type result struct {
		user int64
		err  error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, job := range []struct {
		userID int64
		card   string
		key    string
	}{
		{userA, validIDCardMale2016, "key-cancel-conc-a2"},
		{userB, validIDCardMale2017, "key-cancel-conc-b1"},
	} {
		wg.Add(1)
		go func(userID int64, card, key string) {
			defer wg.Done()
			_, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(card, model.DanceTypeClassical, key))
			results <- result{user: userID, err: err}
		}(job.userID, job.card, job.key)
	}
	wg.Wait()
	close(results)

	success := 0
	for r := range results {
		if r.err == nil {
			success++
			continue
		}
		if code := bizCode(t, r.err); code != httpx.CodeQuotaFull {
			t.Fatalf("loser err = %v, want 41005", r.err)
		}
	}
	if success != 1 {
		t.Fatalf("success = %d, want 1（只允许一个抢到释放的名额）", success)
	}
	if took := groupTaken(t, f.db, groupID); took != 1 {
		t.Fatalf("taken = %d, want 1", took)
	}
}

// TestListHistory_IncludesCanceledExcludesPending 历史订单含已取消留档、
// 不含待支付；分页与归属正确。
func TestListHistory_IncludesCanceledExcludesPending(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-history")
	eventID := mustCreateRegisteringEvent(t, f.regFixture, "历史订单赛事", defaultRegGroups)

	classical, err := f.regSvc.Submit(ctx, userID, eventID,
		submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-history-001"))
	if err != nil {
		t.Fatalf("submit classical: %v", err)
	}
	folk, err := f.regSvc.Submit(ctx, userID, eventID,
		submitReq(validIDCardMale2016, model.DanceTypeFolk, "key-history-002"))
	if err != nil {
		t.Fatalf("submit folk: %v", err)
	}
	if _, err := f.svc.Cancel(ctx, userID, folk.OrderID, model.OrderCancelReasonPayFailed); err != nil {
		t.Fatalf("cancel folk: %v", err)
	}

	// 待支付订单不进入历史列表：只剩已取消的 folk。
	resp, err := f.svc.ListHistory(ctx, userID, 1, 20)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if resp.Total != 1 || len(resp.List) != 1 {
		t.Fatalf("history total/list = %d/%d, want 1/1", resp.Total, len(resp.List))
	}
	item := resp.List[0]
	if item.OrderID != folk.OrderID || item.Status != model.OrderStatusCanceled ||
		item.CancelReason == nil || *item.CancelReason != model.OrderCancelReasonPayFailed ||
		item.CanceledAt == nil || item.PaidAt != nil {
		t.Fatalf("canceled item = %+v", item)
	}
	if item.EventID != eventID || item.EventName != "历史订单赛事" ||
		item.LevelGroup != model.GroupLevelU11 || item.DanceType != model.DanceTypeFolk ||
		item.PlayerName != "王小舞" || item.Amount != classical.Amount {
		t.Fatalf("item context = %+v", item)
	}

	// 支付后进入历史列表，按创建时间倒序（folk 在后，排最前）。
	if _, err := f.db.SQL().ExecContext(ctx,
		`UPDATE orders SET status = 'paid', paid_at = '2027-01-01T10:00:00+08:00' WHERE id = ?`,
		classical.OrderID); err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	resp, err = f.svc.ListHistory(ctx, userID, 1, 20)
	if err != nil {
		t.Fatalf("ListHistory after pay: %v", err)
	}
	if resp.Total != 2 || len(resp.List) != 2 {
		t.Fatalf("history total/list = %d/%d, want 2/2", resp.Total, len(resp.List))
	}
	if resp.List[0].OrderID != folk.OrderID || resp.List[1].OrderID != classical.OrderID {
		t.Fatalf("history order = %d,%d, want %d,%d",
			resp.List[0].OrderID, resp.List[1].OrderID, folk.OrderID, classical.OrderID)
	}
	if resp.List[1].Status != model.OrderStatusPaid || resp.List[1].PaidAt == nil ||
		resp.List[1].CancelReason != nil || resp.List[1].CanceledAt != nil {
		t.Fatalf("paid item = %+v", resp.List[1])
	}

	// 分页：page_size=1 只返回一条，total 仍为 2。
	page2, err := f.svc.ListHistory(ctx, userID, 2, 1)
	if err != nil {
		t.Fatalf("ListHistory page 2: %v", err)
	}
	if page2.Total != 2 || len(page2.List) != 1 || page2.Page != 2 || page2.PageSize != 1 ||
		page2.List[0].OrderID != classical.OrderID {
		t.Fatalf("page2 = %+v", page2)
	}

	// 他人看不到该用户的订单。
	otherUser := mustInsertRegUser(t, f.db, "o-history-other")
	other, err := f.svc.ListHistory(ctx, otherUser, 1, 20)
	if err != nil {
		t.Fatalf("other ListHistory: %v", err)
	}
	if other.Total != 0 || len(other.List) != 0 {
		t.Fatalf("other history = %+v, want empty", other)
	}
}
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
