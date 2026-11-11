package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/wxpay"
)

// failingRefunder 是退款接口失败桩。
type failingRefunder struct{ err error }

func (f failingRefunder) Refund(context.Context, wxpay.RefundRequest) (*wxpay.RefundResult, error) {
	return nil, f.err
}

// refundNotifyBody 构造退款回调明文（mock 通道直接解析）。
func refundNotifyBody(orderNo, ticketNo, refundID, status string, refund, total int64) []byte {
	body, _ := json.Marshal(map[string]any{
		"mchid":         "1900000109",
		"out_trade_no":  orderNo,
		"out_refund_no": ticketNo,
		"refund_id":     refundID,
		"refund_status": status,
		"amount":        map[string]any{"total": total, "refund": refund, "currency": "CNY"},
	})
	return body
}

// TestApproveRefundAndCallback 覆盖同意退款 → 退款回调成功的完整状态机与名额释放。
func TestApproveRefundAndCallback(t *testing.T) {
	f := newRefundFixture(t)
	ctx := context.Background()
	_, groupID, orderNo := f.paidOrder(t, "approve")
	f.takeSlot(t, groupID)

	created, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq(orderNo, "伤病退赛"))
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	resp, err := f.svc.Approve(ctx, f.adminID, created.TicketID,
		model.ApproveRefundTicketRequest{Amount: 98000})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if resp.Status != model.RefundTicketStatusApproved || resp.OrderStatus != model.OrderStatusRefunding ||
		resp.RefundID == "" {
		t.Fatalf("approve response = %+v", resp)
	}

	ticket, err := f.stores.RefundTickets.Get(ctx, created.TicketID)
	if err != nil {
		t.Fatalf("load ticket: %v", err)
	}
	if ticket.Status != model.RefundTicketStatusApproved || ticket.HandlerID == nil || ticket.HandledAt == nil {
		t.Fatalf("ticket = %+v", ticket)
	}
	order, err := f.stores.Orders.Get(ctx, ticket.OrderID)
	if err != nil || order.Status != model.OrderStatusRefunding {
		t.Fatalf("order = %+v, err = %v", order, err)
	}
	var refundPayments int
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(1) FROM payments WHERE order_id = ? AND type = 'refund' AND refund_id = ?`,
		order.ID, resp.RefundID).Scan(&refundPayments); err != nil {
		t.Fatalf("query refund payment: %v", err)
	}
	if refundPayments != 1 {
		t.Fatalf("refund payment count = %d, want 1", refundPayments)
	}
	if n := f.countAdminLog(t, adminActionRefundApprove, "refund_ticket", created.TicketID); n != 1 {
		t.Fatalf("approve audit count = %d, want 1", n)
	}

	// 退款成功回调：订单 refunded、报名 withdrawn、名额释放、工单 refunded。
	body := refundNotifyBody(orderNo, created.TicketNo, resp.RefundID, wxpay.RefundStatusSuccess, 98000, 98000)
	if err := f.svc.HandleRefundNotify(ctx, wxpay.NotifyHeaders{}, body); err != nil {
		t.Fatalf("refund notify: %v", err)
	}
	ticket, _ = f.stores.RefundTickets.Get(ctx, created.TicketID)
	if ticket.Status != model.RefundTicketStatusRefunded {
		t.Fatalf("ticket status = %s, want refunded", ticket.Status)
	}
	order, _ = f.stores.Orders.Get(ctx, ticket.OrderID)
	if order.Status != model.OrderStatusRefunded {
		t.Fatalf("order status = %s, want refunded", order.Status)
	}
	registration, err := f.stores.Registrations.Get(ctx, order.RegistrationID)
	if err != nil || registration.Status != model.RegistrationStatusWithdrawn {
		t.Fatalf("registration = %+v, err = %v", registration, err)
	}
	group, err := f.stores.Groups.Get(ctx, groupID)
	if err != nil || group.Taken != 0 {
		t.Fatalf("group taken = %d, err = %v", group.Taken, err)
	}
	var notified, raw string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT notified_at, raw_callback FROM payments WHERE refund_id = ?`, resp.RefundID).
		Scan(&notified, &raw); err != nil {
		t.Fatalf("query refund payment: %v", err)
	}
	if notified == "" || raw == "" {
		t.Fatalf("payment callback info not filled: notified=%q raw=%q", notified, raw)
	}
	if n := f.countAdminLog(t, adminActionRefundNotify, "refund_ticket", created.TicketID); n != 1 {
		t.Fatalf("callback audit count = %d, want 1", n)
	}

	// 重复回调幂等：名额不重复释放，流水仍只有一条。
	if err := f.svc.HandleRefundNotify(ctx, wxpay.NotifyHeaders{}, body); err != nil {
		t.Fatalf("duplicate refund notify: %v", err)
	}
	group, _ = f.stores.Groups.Get(ctx, groupID)
	if group.Taken != 0 {
		t.Fatalf("taken after duplicate callback = %d, want 0", group.Taken)
	}
	var totalRefundPayments int
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(1) FROM payments WHERE order_id = ? AND type = 'refund'`, order.ID).
		Scan(&totalRefundPayments); err != nil {
		t.Fatalf("count refund payments: %v", err)
	}
	if totalRefundPayments != 1 {
		t.Fatalf("refund payment count after duplicate = %d, want 1", totalRefundPayments)
	}
}

// TestApproveRefundAmountRules 覆盖金额上限与退费政策默认金额/调整原因。
func TestApproveRefundAmountRules(t *testing.T) {
	f := newRefundFixture(t)
	ctx := context.Background()
	eventID, _, orderNo := f.paidOrder(t, "amount")
	created, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq(orderNo, "个人原因"))
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	if _, err := f.svc.Approve(ctx, f.adminID, created.TicketID,
		model.ApproveRefundTicketRequest{Amount: 98001}); mustBizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("over paid amount: %v", err)
	}

	// 抽签后默认 50%（49000）；按全额退需填调整原因。
	f.setEventTimelinePast(t, eventID)
	if _, err := f.svc.Approve(ctx, f.adminID, created.TicketID,
		model.ApproveRefundTicketRequest{Amount: 98000}); mustBizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("adjusted without reason: %v", err)
	}
	resp, err := f.svc.Approve(ctx, f.adminID, created.TicketID,
		model.ApproveRefundTicketRequest{Amount: 98000, AdjustReason: "伤病凭诊断证明全额退"})
	if err != nil {
		t.Fatalf("approve with adjust reason: %v", err)
	}
	if resp.TicketNo != created.TicketNo {
		t.Fatalf("response = %+v", resp)
	}
}

// TestApproveRefundAPIFailureAndRetry 覆盖退款接口失败回退与重试。
func TestApproveRefundAPIFailureAndRetry(t *testing.T) {
	f := newRefundFixture(t)
	ctx := context.Background()
	_, _, orderNo := f.paidOrder(t, "fail")
	created, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq(orderNo, "原因"))
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	f.svc.refunder = failingRefunder{err: fmt.Errorf("upstream timeout")}
	_, err = f.svc.Approve(ctx, f.adminID, created.TicketID, model.ApproveRefundTicketRequest{Amount: 98000})
	if code := mustBizCode(t, err); code != httpx.CodeRefundAPIFailed {
		t.Fatalf("code = %d, want 41014", code)
	}
	ticket, _ := f.stores.RefundTickets.Get(ctx, created.TicketID)
	if ticket.Status != model.RefundTicketStatusRefundFailed {
		t.Fatalf("ticket status = %s, want refund_failed", ticket.Status)
	}
	order, _ := f.stores.Orders.Get(ctx, ticket.OrderID)
	if order.Status != model.OrderStatusPaid {
		t.Fatalf("order status = %s, want paid", order.Status)
	}
	if n := f.countAdminLog(t, adminActionRefundFailed, "refund_ticket", created.TicketID); n != 1 {
		t.Fatalf("failed audit count = %d, want 1", n)
	}

	// 重试：refund_failed 可再次同意。
	f.svc.refunder = f.gateway
	if _, err := f.svc.Approve(ctx, f.adminID, created.TicketID,
		model.ApproveRefundTicketRequest{Amount: 98000}); err != nil {
		t.Fatalf("retry approve: %v", err)
	}
	ticket, _ = f.stores.RefundTickets.Get(ctx, created.TicketID)
	if ticket.Status != model.RefundTicketStatusApproved {
		t.Fatalf("ticket status after retry = %s, want approved", ticket.Status)
	}
}

// TestRejectRefund 覆盖驳回：参数、状态机与审计。
func TestRejectRefund(t *testing.T) {
	f := newRefundFixture(t)
	ctx := context.Background()
	_, _, orderNo := f.paidOrder(t, "reject")
	created, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq(orderNo, "原因"))
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	if _, err := f.svc.Reject(ctx, f.adminID, created.TicketID,
		model.RejectRefundTicketRequest{}); mustBizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("empty reason: %v", err)
	}
	resp, err := f.svc.Reject(ctx, f.adminID, created.TicketID,
		model.RejectRefundTicketRequest{RejectReason: "已过退费期"})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if resp.Status != model.RefundTicketStatusRejected || resp.OrderStatus != model.OrderStatusPaid {
		t.Fatalf("response = %+v", resp)
	}
	ticket, _ := f.stores.RefundTickets.Get(ctx, created.TicketID)
	if ticket.Status != model.RefundTicketStatusRejected || ticket.RejectReason == nil {
		t.Fatalf("ticket = %+v", ticket)
	}
	if n := f.countAdminLog(t, adminActionRefundReject, "refund_ticket", created.TicketID); n != 1 {
		t.Fatalf("reject audit count = %d, want 1", n)
	}

	// 重复驳回 / 驳回后同意 → 41013。
	if _, err := f.svc.Reject(ctx, f.adminID, created.TicketID,
		model.RejectRefundTicketRequest{RejectReason: "再次驳回"}); mustBizCode(t, err) != httpx.CodeWaitlistStateDenied {
		t.Fatalf("second reject: %v", err)
	}
	if _, err := f.svc.Approve(ctx, f.adminID, created.TicketID,
		model.ApproveRefundTicketRequest{Amount: 98000}); mustBizCode(t, err) != httpx.CodeWaitlistStateDenied {
		t.Fatalf("approve rejected ticket: %v", err)
	}
}

// TestRefundNotifyUnknownTicket 未知工单的成功回调返回错误（微信重推），
// 流水中不存在该 refund_id。
func TestRefundNotifyUnknownTicket(t *testing.T) {
	f := newRefundFixture(t)
	body := refundNotifyBody("ORD-none", "TK-none", "refund-none", wxpay.RefundStatusSuccess, 1, 1)
	if err := f.svc.HandleRefundNotify(context.Background(), wxpay.NotifyHeaders{}, body); err == nil {
		t.Fatal("expected error for unknown ticket")
	}
}
