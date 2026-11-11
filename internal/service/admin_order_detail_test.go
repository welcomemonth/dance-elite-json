package service

import (
	"context"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// TestAdminOrderDetail 覆盖订单详情：支付/退款流水、工单关联、资料快照与操作记录。
func TestAdminOrderDetail(t *testing.T) {
	f := newAdminQueryFixture(t)
	ctx := context.Background()
	u11 := mustGroupID(t, f.db, f.eventID, model.GroupLevelU11, model.DanceTypeClassical)
	s := f.seed(t, "D1", "ORD-DETAIL-1", model.OrderStatusRefunded,
		model.OrderSourceMiniapp, model.DanceTypeClassical, u11, "详情选手")
	f.setUserPhone(t, s.UserID, "13800000001")
	f.addPayPayment(t, s.OrderNo, "4200PAYDETAIL")
	f.fillMaterials(t, s.RegistrationID, allMaterialFields...)
	mustExecSQL(t, f.db, `UPDATE registrations SET contact_phone = '13800000001' WHERE id = ?`, s.RegistrationID)

	// 退款流水 + 退款工单 + 客服工单（订单关联与报名单关联各一条）。
	mustExecSQL(t, f.db, `
		INSERT INTO payments (order_id, type, refund_id, amount, status, notified_at)
		VALUES ((SELECT id FROM orders WHERE order_no = ?), 'refund', 'REF-DETAIL', 98000,
		        'success', '2027-01-05T12:00:00+08:00')`, s.OrderNo)
	mustExecSQL(t, f.db, `
		INSERT INTO refund_tickets (ticket_no, order_id, reason, applicant, status,
			handler_id, handled_at, created_by, created_at)
		VALUES ('RF-DETAIL-1', (SELECT id FROM orders WHERE order_no = ?),
		        '伤病退赛', '客服代录', 'approved', 1, '2027-01-05T11:00:00+08:00', 1,
		        '2027-01-03T09:00:00+08:00')`, s.OrderNo)
	mustExecSQL(t, f.db, `
		INSERT INTO cs_tickets (type, user_name, phone, content, related_order_id, status, created_by)
		VALUES ('refund', '详情选手家长', '13800000001', '申请退款',
		        (SELECT id FROM orders WHERE order_no = ?), 'done', 1)`, s.OrderNo)
	mustExecSQL(t, f.db, `
		INSERT INTO cs_tickets (type, phone, content, related_registration_id, status, created_by)
		VALUES ('other', '13800000001', '人工报名跟进', ?, 'processing', 1)`, s.RegistrationID)

	resp, err := f.orderSvc.Detail(ctx, s.OrderNo)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	// 订单/报名：选手以 player_no + id_type 标识，联系电话回退用户手机号。
	if resp.Order.OrderNo != s.OrderNo || resp.Order.Status != model.OrderStatusRefunded ||
		resp.Order.PlayerNo != s.PlayerNo || resp.Order.IDType != "id_card" ||
		resp.Order.TransactionID == nil || *resp.Order.TransactionID != "4200PAYDETAIL" {
		t.Fatalf("order = %+v", resp.Order)
	}
	if resp.Registration.RegistrationID != s.RegistrationID ||
		resp.Registration.Level != model.GroupLevelU11 ||
		resp.Registration.ContactPhone == nil || *resp.Registration.ContactPhone != "13800000001" {
		t.Fatalf("registration = %+v", resp.Registration)
	}

	// 支付/退款流水。
	if len(resp.Payments) != 2 || resp.Payments[0].Type != model.PaymentTypePay ||
		resp.Payments[1].Type != model.PaymentTypeRefund ||
		resp.Payments[1].RefundID == nil || *resp.Payments[1].RefundID != "REF-DETAIL" {
		t.Fatalf("payments = %+v", resp.Payments)
	}

	// 退款工单摘要（含处理人）。
	if len(resp.RefundTickets) != 1 {
		t.Fatalf("refund tickets = %+v", resp.RefundTickets)
	}
	ticket := resp.RefundTickets[0]
	if ticket.TicketNo != "RF-DETAIL-1" || ticket.Status != model.RefundTicketStatusApproved ||
		ticket.HandlerName == nil || ticket.HandledAt == nil {
		t.Fatalf("refund ticket = %+v", ticket)
	}

	// 客服工单：订单关联 + 报名单关联均返回。
	if len(resp.CSTickets) != 2 {
		t.Fatalf("cs tickets = %+v", resp.CSTickets)
	}

	// 资料快照与完成度。
	if resp.MaterialStatus.Total != 8 || resp.MaterialStatus.Completed != 8 ||
		resp.Materials.PhotoURL == nil || resp.Materials.ProgramName == nil {
		t.Fatalf("materials = %+v / %+v", resp.MaterialStatus, resp.Materials)
	}
	if resp.Materials.Readonly {
		t.Fatal("materials should not be readonly before material_check_at")
	}

	// 操作记录：创建、支付、退款申请、退款同意，按时间升序。
	actions := make([]string, 0, len(resp.OperationLogs))
	for _, log := range resp.OperationLogs {
		actions = append(actions, log.Action)
	}
	want := []string{"created", "paid", "refund_requested", "refund_approved"}
	if len(actions) != len(want) {
		t.Fatalf("operation actions = %v, want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Fatalf("operation actions = %v, want %v", actions, want)
		}
	}

	// 资料截止后只读。
	mustExecSQL(t, f.db, `UPDATE events SET
		reg_start_at = '2018-12-01T00:00:00+08:00',
		reg_end_at = '2018-12-15T00:00:00+08:00',
		lottery_at = '2019-01-01T00:00:00+08:00',
		material_check_at = '2019-02-01T00:00:00+08:00',
		start_at = '2019-03-01T00:00:00+08:00' WHERE id = ?`, f.eventID)
	resp, err = f.orderSvc.Detail(ctx, s.OrderNo)
	if err != nil {
		t.Fatalf("detail after deadline: %v", err)
	}
	if !resp.Materials.Readonly {
		t.Fatal("materials should be readonly after material_check_at")
	}

	// 不存在 → 40401；空订单号 → 40001。
	if _, err := f.orderSvc.Detail(ctx, "ORD-MISSING"); mustBizCode(t, err) != 40401 {
		t.Fatalf("missing order should be 40401, got %v", err)
	}
	if _, err := f.orderSvc.Detail(ctx, "  "); mustBizCode(t, err) != 40001 {
		t.Fatalf("blank order_no should be 40001, got %v", err)
	}
}
