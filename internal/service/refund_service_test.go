package service

import (
	"context"
	"strings"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
)

// TestCreateRefundTicket 覆盖工单创建成功路径：pending、登记人、默认来源与审计日志。
func TestCreateRefundTicket(t *testing.T) {
	f := newRefundFixture(t)
	_, _, orderNo := f.paidOrder(t, "create")

	resp, err := f.svc.CreateTicket(context.Background(), f.adminID, createTicketReq(orderNo, "孩子伤病需退赛"))
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if resp.TicketID == 0 || !strings.HasPrefix(resp.TicketNo, "TK") || resp.Status != model.RefundTicketStatusPending {
		t.Fatalf("response = %+v", resp)
	}

	ticket, err := f.stores.RefundTickets.Get(context.Background(), resp.TicketID)
	if err != nil {
		t.Fatalf("load ticket: %v", err)
	}
	if ticket.Status != model.RefundTicketStatusPending || ticket.Reason != "孩子伤病需退赛" {
		t.Fatalf("ticket = %+v", ticket)
	}
	if ticket.Applicant == nil || *ticket.Applicant != "客服代录" {
		t.Fatalf("applicant = %v", ticket.Applicant)
	}
	if ticket.CreatedBy == nil || *ticket.CreatedBy != f.adminID {
		t.Fatalf("created_by = %v, want %d", ticket.CreatedBy, f.adminID)
	}
	if n := f.countAdminLog(t, adminActionRefundCreate, "refund_ticket", ticket.ID); n != 1 {
		t.Fatalf("create audit log count = %d, want 1", n)
	}
}

// TestCreateRefundTicketValidation 覆盖退款规则与参数校验的错误码。
func TestCreateRefundTicketValidation(t *testing.T) {
	f := newRefundFixture(t)
	ctx := context.Background()

	if _, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq("", "原因")); mustBizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("empty order_no: %v", err)
	}
	if _, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq("ORD-x", "")); mustBizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("empty reason: %v", err)
	}
	if _, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq("ORD-x", strings.Repeat("长", 501))); mustBizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("overlong reason: %v", err)
	}
	if _, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq("ORD-missing", "原因")); mustBizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("missing order: %v", err)
	}

	// 非已支付订单 → 41015。
	_, _, canceledNo := f.paidOrder(t, "canceled")
	f.setOrderStatus(t, canceledNo, model.OrderStatusCanceled)
	if _, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq(canceledNo, "原因")); mustBizCode(t, err) != httpx.CodeRefundNotAllowed {
		t.Fatalf("canceled order: %v", err)
	}
	_, _, refundedNo := f.paidOrder(t, "refunded")
	f.setOrderStatus(t, refundedNo, model.OrderStatusRefunded)
	if _, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq(refundedNo, "原因")); mustBizCode(t, err) != httpx.CodeRefundNotAllowed {
		t.Fatalf("refunded order: %v", err)
	}

	// 已过报名截止 → 41015。
	eventID, _, expiredNo := f.paidOrder(t, "expired")
	f.setEventRegEnd(t, eventID, "2020-01-01T00:00:00+08:00")
	if _, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq(expiredNo, "原因")); mustBizCode(t, err) != httpx.CodeRefundNotAllowed {
		t.Fatalf("after reg end: %v", err)
	}

	// 同订单重复登记 → 40901；首次应成功。
	_, _, dupNo := f.paidOrder(t, "dup")
	if _, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq(dupNo, "原因")); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := f.svc.CreateTicket(ctx, f.adminID, createTicketReq(dupNo, "原因")); mustBizCode(t, err) != httpx.CodeConflict {
		t.Fatalf("duplicate create: %v", err)
	}
}

// TestCreateRefundTicketAfterRejected 驳回后若仍在报名期内可重新登记。
func TestCreateRefundTicketAfterRejected(t *testing.T) {
	f := newRefundFixture(t)
	_, _, orderNo := f.paidOrder(t, "rejected")

	first, err := f.svc.CreateTicket(context.Background(), f.adminID, createTicketReq(orderNo, "原因"))
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	mustExecSQL(t, f.db, `UPDATE refund_tickets SET status = ? WHERE id = ?`,
		model.RefundTicketStatusRejected, first.TicketID)

	second, err := f.svc.CreateTicket(context.Background(), f.adminID, createTicketReq(orderNo, "再次申请"))
	if err != nil {
		t.Fatalf("re-create after reject: %v", err)
	}
	if second.TicketID == first.TicketID {
		t.Fatalf("expected new ticket, got same id %d", second.TicketID)
	}
}
