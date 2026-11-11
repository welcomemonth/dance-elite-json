package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/wxpay"
)

// fakeNotifier 是可注入结果的回调解析替身。
type fakeNotifier struct {
	tx    *wxpay.Transaction
	err   error
	calls int
}

func (f *fakeNotifier) ParseNotify(_ wxpay.NotifyHeaders, _ []byte) (*wxpay.Transaction, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.tx, nil
}

// fakeQuerier 是可注入结果的主动查单替身。
type fakeQuerier struct {
	tx    *wxpay.Transaction
	err   error
	calls int
}

func (f *fakeQuerier) QueryOrder(_ context.Context, _ string) (*wxpay.Transaction, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.tx, nil
}

// paymentFixture 复用 T10 订单夹具并装配 PaymentService。
type paymentFixture struct {
	*orderFixture
	notifier *fakeNotifier
	querier  *fakeQuerier
	paySvc   *PaymentService
}

func newPaymentFixture(t *testing.T) *paymentFixture {
	t.Helper()
	f := newOrderFixture(t)
	notifier := &fakeNotifier{}
	querier := &fakeQuerier{}
	return &paymentFixture{
		orderFixture: f,
		notifier:     notifier,
		querier:      querier,
		paySvc:       NewPaymentService(f.stores, notifier, querier),
	}
}

// mustPendingOrder 走 T9 提交报名产生订单，并回读订单实体。
func (f *paymentFixture) mustPendingOrder(t *testing.T, userID int64, key string) *model.Order {
	t.Helper()
	orderID := mustSubmitOrder(t, f.orderFixture, userID, key)
	order, err := f.stores.Orders.Get(context.Background(), orderID)
	if err != nil {
		t.Fatalf("load order %d: %v", orderID, err)
	}
	return order
}

// mustRegistrationStatus 读取报名单状态。
func mustRegistrationStatus(t *testing.T, f *paymentFixture, id int64) string {
	t.Helper()
	var status string
	if err := f.db.SQL().QueryRow(`SELECT status FROM registrations WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("query registration status: %v", err)
	}
	return status
}

// countPayments 返回订单的支付流水条数。
func countPayments(t *testing.T, f *paymentFixture, orderID int64) int {
	t.Helper()
	var n int
	if err := f.db.SQL().QueryRow(`SELECT COUNT(*) FROM payments WHERE order_id = ?`, orderID).Scan(&n); err != nil {
		t.Fatalf("count payments: %v", err)
	}
	return n
}

// successTx 构造与订单匹配的成功交易。
func successTx(order *model.Order) *wxpay.Transaction {
	return &wxpay.Transaction{
		OutTradeNo:    order.OrderNo,
		TransactionID: "wx-tx-" + order.OrderNo,
		TradeState:    wxpay.TradeStateSuccess,
		AmountTotal:   order.Amount,
		SuccessTime:   "2027-01-01T10:00:00+08:00",
	}
}

func TestHandleNotify_Success(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-pay-ok")
	order := f.mustPendingOrder(t, userID, "key-pay-ok-1")

	// 先预下单，验证流水会关联 prepay_id。
	if _, err := f.svc.Prepay(ctx, userID, "o-openid", order.ID); err != nil {
		t.Fatalf("prepay: %v", err)
	}

	tx := successTx(order)
	f.notifier.tx = tx
	raw := []byte(`{"id":"ev-1","event_type":"TRANSACTION.SUCCESS"}`)
	if err := f.paySvc.HandleNotify(ctx, wxpay.NotifyHeaders{}, raw); err != nil {
		t.Fatalf("HandleNotify: %v", err)
	}

	// 订单 → paid + paid_at；报名单 → active。
	updated, err := f.stores.Orders.Get(ctx, order.ID)
	if err != nil {
		t.Fatalf("reload order: %v", err)
	}
	if updated.Status != model.OrderStatusPaid || updated.PaidAt == nil || *updated.PaidAt != "2027-01-01T10:00:00+08:00" {
		t.Fatalf("order = %+v", updated)
	}
	if status := mustRegistrationStatus(t, f, order.RegistrationID); status != model.RegistrationStatusActive {
		t.Fatalf("registration status = %q, want active", status)
	}

	// 流水：type=pay/status=success/金额与 gateway 单号、原始报文留档。
	var (
		payType, payStatus, rawCallback, notifiedAt string
		amount                                      int64
		transactionID, prepayID                     string
	)
	if err := f.db.SQL().QueryRow(`
		SELECT type, status, amount, transaction_id, prepay_id, raw_callback, notified_at
		FROM payments WHERE order_id = ?`, order.ID).
		Scan(&payType, &payStatus, &amount, &transactionID, &prepayID, &rawCallback, &notifiedAt); err != nil {
		t.Fatalf("query payment: %v", err)
	}
	if payType != model.PaymentTypePay || payStatus != model.PaymentStatusSuccess {
		t.Fatalf("payment type/status = %s/%s", payType, payStatus)
	}
	if amount != order.Amount || transactionID != tx.TransactionID || prepayID != "fake-prepay-1" {
		t.Fatalf("payment amount/tx/prepay = %d/%s/%s", amount, transactionID, prepayID)
	}
	if rawCallback != string(raw) || notifiedAt == "" {
		t.Fatalf("raw_callback = %q, notified_at = %q", rawCallback, notifiedAt)
	}
}

func TestHandleNotify_Idempotent(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-pay-idem")
	order := f.mustPendingOrder(t, userID, "key-pay-idem1")
	f.notifier.tx = successTx(order)

	for i := 0; i < 2; i++ {
		if err := f.paySvc.HandleNotify(ctx, wxpay.NotifyHeaders{}, []byte(`{"id":"ev-1"}`)); err != nil {
			t.Fatalf("HandleNotify #%d: %v", i+1, err)
		}
	}
	if n := countPayments(t, f, order.ID); n != 1 {
		t.Fatalf("payments = %d, want 1（重复回调只落一条流水）", n)
	}
	if updated, _ := f.stores.Orders.Get(ctx, order.ID); updated.Status != model.OrderStatusPaid {
		t.Fatalf("order status = %q", updated.Status)
	}
}

func TestHandleNotify_AmountMismatchRejected(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-pay-amt")
	order := f.mustPendingOrder(t, userID, "key-pay-amt-1")

	tx := successTx(order)
	tx.AmountTotal = order.Amount - 1
	f.notifier.tx = tx
	err := f.paySvc.HandleNotify(ctx, wxpay.NotifyHeaders{}, []byte(`{"id":"ev-amt"}`))
	if !errors.Is(err, ErrPaymentAmountMismatch) {
		t.Fatalf("err = %v, want ErrPaymentAmountMismatch", err)
	}
	updated, _ := f.stores.Orders.Get(ctx, order.ID)
	if updated.Status != model.OrderStatusPendingPayment {
		t.Fatalf("order status = %q, want pending_payment", updated.Status)
	}
	if n := countPayments(t, f, order.ID); n != 0 {
		t.Fatalf("payments = %d, want 0", n)
	}
}

// TestHandleNotify_RollbackWhenRegistrationNotPending 验证"流水写入后订单/报名
// 更新失败"整事务回滚：报名单已被置为取消时，支付回调不得留下任何半成品。
func TestHandleNotify_RollbackWhenRegistrationNotPending(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-pay-rollback")
	order := f.mustPendingOrder(t, userID, "key-pay-rb-1")

	if _, err := f.db.SQL().ExecContext(ctx,
		`UPDATE registrations SET status = 'canceled' WHERE id = ?`, order.RegistrationID); err != nil {
		t.Fatalf("cancel registration: %v", err)
	}
	f.notifier.tx = successTx(order)
	if err := f.paySvc.HandleNotify(ctx, wxpay.NotifyHeaders{}, []byte(`{"id":"ev-rb"}`)); err == nil {
		t.Fatal("HandleNotify should fail when registration is not pending")
	}

	if n := countPayments(t, f, order.ID); n != 0 {
		t.Fatalf("payments = %d, want 0（事务回滚）", n)
	}
	updated, _ := f.stores.Orders.Get(ctx, order.ID)
	if updated.Status != model.OrderStatusPendingPayment {
		t.Fatalf("order status = %q, want pending_payment（回滚）", updated.Status)
	}
}

func TestHandleNotify_NonSuccessAcked(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-pay-notpay")
	order := f.mustPendingOrder(t, userID, "key-pay-np-1")

	f.notifier.tx = &wxpay.Transaction{
		OutTradeNo: order.OrderNo, TransactionID: "wx-tx-np",
		TradeState: wxpay.TradeStateNotPay, AmountTotal: order.Amount,
	}
	if err := f.paySvc.HandleNotify(ctx, wxpay.NotifyHeaders{}, []byte(`{}`)); err != nil {
		t.Fatalf("HandleNotify: %v", err)
	}
	if n := countPayments(t, f, order.ID); n != 0 {
		t.Fatalf("payments = %d, want 0", n)
	}
}

func TestHandleNotify_InvalidOrder(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	f.notifier.tx = &wxpay.Transaction{
		OutTradeNo: "no-such-order", TransactionID: "wx-tx-missing",
		TradeState: wxpay.TradeStateSuccess, AmountTotal: 1,
	}
	if err := f.paySvc.HandleNotify(ctx, wxpay.NotifyHeaders{}, []byte(`{}`)); err == nil {
		t.Fatal("unknown order should fail so WeChat retries")
	}

	// 验签失败原样上抛（路由层映射 HTTP 401）。
	f.notifier.err = errors.Join(wxpay.ErrInvalidNotify, errors.New("bad signature"))
	f.notifier.tx = nil
	if err := f.paySvc.HandleNotify(ctx, wxpay.NotifyHeaders{}, nil); !errors.Is(err, wxpay.ErrInvalidNotify) {
		t.Fatalf("err = %v, want ErrInvalidNotify", err)
	}
}

func TestQueryStatus_PaidViaQueryFallback(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-query-hit")
	order := f.mustPendingOrder(t, userID, "key-query-hit")

	f.querier.tx = successTx(order)
	resp, err := f.paySvc.QueryStatus(ctx, userID, order.ID)
	if err != nil {
		t.Fatalf("QueryStatus: %v", err)
	}
	if resp.Status != model.OrderStatusPaid || resp.PaidAt == nil {
		t.Fatalf("resp = %+v", resp)
	}
	if status := mustRegistrationStatus(t, f, order.RegistrationID); status != model.RegistrationStatusActive {
		t.Fatalf("registration status = %q", status)
	}
	if n := countPayments(t, f, order.ID); n != 1 {
		t.Fatalf("payments = %d, want 1", n)
	}
	// 查单兜底写入的流水 raw_callback 为空（来源可区分）。
	var raw sql.NullString
	if err := f.db.SQL().QueryRow(`SELECT raw_callback FROM payments WHERE order_id = ?`, order.ID).Scan(&raw); err != nil {
		t.Fatalf("query raw_callback: %v", err)
	}
	if raw.Valid {
		t.Fatalf("raw_callback = %q, want NULL for query fallback", raw.String)
	}
}

func TestQueryStatus_NotPaidStaysPending(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-query-np")
	order := f.mustPendingOrder(t, userID, "key-query-np")

	f.querier.tx = &wxpay.Transaction{
		OutTradeNo: order.OrderNo, TradeState: wxpay.TradeStateNotPay,
	}
	resp, err := f.paySvc.QueryStatus(ctx, userID, order.ID)
	if err != nil {
		t.Fatalf("QueryStatus: %v", err)
	}
	if resp.Status != model.OrderStatusPendingPayment {
		t.Fatalf("status = %q, want pending_payment", resp.Status)
	}
	if n := countPayments(t, f, order.ID); n != 0 {
		t.Fatalf("payments = %d, want 0", n)
	}
}

func TestQueryStatus_AlreadyPaidSkipsQuery(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-query-paid")
	order := f.mustPendingOrder(t, userID, "key-query-pd")

	if _, err := f.db.SQL().ExecContext(ctx,
		`UPDATE orders SET status = 'paid', paid_at = '2027-01-01T10:00:00+08:00' WHERE id = ?`, order.ID); err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	resp, err := f.paySvc.QueryStatus(ctx, userID, order.ID)
	if err != nil {
		t.Fatalf("QueryStatus: %v", err)
	}
	if resp.Status != model.OrderStatusPaid || f.querier.calls != 0 {
		t.Fatalf("status = %q, querier calls = %d", resp.Status, f.querier.calls)
	}
}

func TestQueryStatus_Errors(t *testing.T) {
	f := newPaymentFixture(t)
	ctx := context.Background()
	userID := mustInsertRegUser(t, f.db, "o-query-err")
	order := f.mustPendingOrder(t, userID, "key-query-err")

	if _, err := f.paySvc.QueryStatus(ctx, userID, 9999); err == nil || bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("missing order err = %v, want 40401", err)
	}
	other := mustInsertRegUser(t, f.db, "o-query-other")
	if _, err := f.paySvc.QueryStatus(ctx, other, order.ID); err == nil || bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("other user err = %v, want 40401", err)
	}

	f.querier.err = errors.New("wechat unavailable")
	if _, err := f.paySvc.QueryStatus(ctx, userID, order.ID); err == nil || bizCode(t, err) != httpx.CodeInternal {
		t.Fatalf("query failure err = %v, want 50001", err)
	}
}
