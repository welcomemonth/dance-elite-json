package service

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// mustInsertAdmin 直接插入一条后台账号并返回其 ID（列表角色隔离测试用）。
func mustInsertAdmin(t *testing.T, f *regFixture, username, role string) int64 {
	t.Helper()
	res, err := f.db.SQL().ExecContext(context.Background(),
		`INSERT INTO admins (username, password_hash, role) VALUES (?, 'x', ?)`, username, role)
	if err != nil {
		t.Fatalf("insert admin %s: %v", username, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("admin last insert id: %v", err)
	}
	return id
}

// createCSTicket 通过服务登记工单（测试前置）。
func createCSTicket(t *testing.T, svc *CSTicketService, adminID int64, req model.CreateCSTicketRequest) int64 {
	t.Helper()
	resp, err := svc.Create(context.Background(), adminID, req)
	if err != nil {
		t.Fatalf("create cs ticket: %v", err)
	}
	return resp.TicketID
}

// TestCSTicketCreate 覆盖登记接口的关联订单、审计日志与必填校验。
func TestCSTicketCreate(t *testing.T) {
	f := newRefundFixture(t)
	svc := NewCSTicketService(f.stores)
	_, _, orderNo := f.paidOrder(t, "cs-create")

	resp, err := svc.Create(context.Background(), f.adminID, model.CreateCSTicketRequest{
		Type:           model.CSTicketTypeRefund,
		UserName:       "张小雨",
		Phone:          "13812345678",
		Content:        "孩子伤病需退赛，申请退款",
		RelatedOrderNo: orderNo,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.TicketID <= 0 || resp.Status != model.CSTicketStatusPending {
		t.Fatalf("create resp = %+v", resp)
	}

	var relatedOrderID sql.NullInt64
	if err := f.db.SQL().QueryRowContext(context.Background(),
		`SELECT related_order_id FROM cs_tickets WHERE id = ?`, resp.TicketID).Scan(&relatedOrderID); err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if !relatedOrderID.Valid || relatedOrderID.Int64 <= 0 {
		t.Fatal("related_order_id should be resolved from related_order_no")
	}
	if n := f.countAdminLog(t, "admin.cs_ticket.create", "cs_ticket", resp.TicketID); n != 1 {
		t.Fatalf("create admin logs = %d, want 1", n)
	}

	cases := []struct {
		name string
		req  model.CreateCSTicketRequest
		code int
	}{
		{"非法类型", model.CreateCSTicketRequest{Type: "bogus", Phone: "138", Content: "内容"}, 40001},
		{"缺电话", model.CreateCSTicketRequest{Type: model.CSTicketTypeOther, Content: "内容"}, 40001},
		{"缺内容", model.CreateCSTicketRequest{Type: model.CSTicketTypeOther, Phone: "138"}, 40001},
		{"内容超长", model.CreateCSTicketRequest{
			Type: model.CSTicketTypeOther, Phone: "138", Content: strings.Repeat("长", 1001)}, 40001},
		{"关联订单不存在", model.CreateCSTicketRequest{
			Type: model.CSTicketTypeWaitlist, Phone: "138", Content: "内容", RelatedOrderNo: "ORD-missing"}, 40401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), f.adminID, tc.req)
			if code := mustBizCode(t, err); code != tc.code {
				t.Fatalf("code = %d, want %d", code, tc.code)
			}
		})
	}
}

// TestCSTicketListRoleScope 覆盖列表筛选与角色隔离：cs 只看自己登记的，admin 看全量。
func TestCSTicketListRoleScope(t *testing.T) {
	f := newRefundFixture(t)
	svc := NewCSTicketService(f.stores)
	csAdminID := mustInsertAdmin(t, f.regFixture, "cs01", model.RoleCS)

	csTicketID := createCSTicket(t, svc, csAdminID, model.CreateCSTicketRequest{
		Type: model.CSTicketTypeNoIDCard, Phone: "13800000001", Content: "外籍选手报名咨询",
	})
	adminTicketID := createCSTicket(t, svc, f.adminID, model.CreateCSTicketRequest{
		Type: model.CSTicketTypeRefund, Phone: "13800000002", Content: "退款咨询",
	})

	csList, err := svc.List(context.Background(), csAdminID, model.RoleCS, model.AdminCSTicketListQuery{})
	if err != nil {
		t.Fatalf("list as cs: %v", err)
	}
	if csList.Total != 1 || len(csList.List) != 1 || csList.List[0].TicketID != csTicketID {
		t.Fatalf("cs list = %+v, want only own ticket %d", csList, csTicketID)
	}

	adminList, err := svc.List(context.Background(), f.adminID, model.RoleAdmin, model.AdminCSTicketListQuery{})
	if err != nil {
		t.Fatalf("list as admin: %v", err)
	}
	if adminList.Total != 2 || len(adminList.List) != 2 {
		t.Fatalf("admin list total = %d, want 2", adminList.Total)
	}
	if adminList.List[0].TicketID != adminTicketID {
		t.Fatalf("admin list should be newest first, got %d", adminList.List[0].TicketID)
	}
	if adminList.List[0].CreatedByName == nil || *adminList.List[0].CreatedByName != "admin" {
		t.Fatalf("created_by_name = %v, want admin", adminList.List[0].CreatedByName)
	}

	filtered, err := svc.List(context.Background(), f.adminID, model.RoleAdmin, model.AdminCSTicketListQuery{
		Type: model.CSTicketTypeRefund,
	})
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if filtered.Total != 1 || filtered.List[0].TicketID != adminTicketID {
		t.Fatalf("type filter = %+v, want ticket %d", filtered, adminTicketID)
	}

	if _, err := svc.List(context.Background(), f.adminID, model.RoleAdmin, model.AdminCSTicketListQuery{
		Status: "bogus",
	}); mustBizCode(t, err) != 40001 {
		t.Fatalf("invalid status filter should be 40001, got %v", err)
	}
	if _, err := svc.List(context.Background(), f.adminID, model.RoleAdmin, model.AdminCSTicketListQuery{
		Type: "bogus",
	}); mustBizCode(t, err) != 40001 {
		t.Fatalf("invalid type filter should be 40001, got %v", err)
	}
}

// TestCSTicketUpdate 覆盖 pending → processing → done 流转、备注与非法流转。
func TestCSTicketUpdate(t *testing.T) {
	f := newRefundFixture(t)
	svc := NewCSTicketService(f.stores)
	ticketID := createCSTicket(t, svc, f.adminID, model.CreateCSTicketRequest{
		Type: model.CSTicketTypeOther, Phone: "13812345678", Content: "咨询比赛时间",
	})
	ctx := context.Background()

	resp, err := svc.Update(ctx, f.adminID, ticketID, model.UpdateCSTicketRequest{
		Status: model.CSTicketStatusProcessing, Remark: "已联系用户",
	})
	if err != nil || resp.Status != model.CSTicketStatusProcessing {
		t.Fatalf("pending -> processing: resp=%+v err=%v", resp, err)
	}
	if n := f.countAdminLog(t, "admin.cs_ticket.update", "cs_ticket", ticketID); n != 1 {
		t.Fatalf("update admin logs = %d, want 1", n)
	}

	// 回退流转不允许。
	if _, err := svc.Update(ctx, f.adminID, ticketID, model.UpdateCSTicketRequest{
		Status: model.CSTicketStatusPending,
	}); mustBizCode(t, err) != 41013 {
		t.Fatalf("backward transition should be 41013, got %v", err)
	}

	if _, err := svc.Update(ctx, f.adminID, ticketID, model.UpdateCSTicketRequest{
		Status: model.CSTicketStatusDone, Remark: "已答复",
	}); err != nil {
		t.Fatalf("processing -> done: %v", err)
	}
	// 同状态幂等更新备注。
	if _, err := svc.Update(ctx, f.adminID, ticketID, model.UpdateCSTicketRequest{
		Status: model.CSTicketStatusDone, Remark: "补充：已归档",
	}); err != nil {
		t.Fatalf("same status remark update: %v", err)
	}
	var status string
	var remark sql.NullString
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT status, remark FROM cs_tickets WHERE id = ?`, ticketID).Scan(&status, &remark); err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if status != model.CSTicketStatusDone || !remark.Valid || remark.String != "补充：已归档" {
		t.Fatalf("ticket = %s / %v, want done / 补充：已归档", status, remark)
	}
	// done 之后不允许再流转。
	if _, err := svc.Update(ctx, f.adminID, ticketID, model.UpdateCSTicketRequest{
		Status: model.CSTicketStatusProcessing,
	}); mustBizCode(t, err) != 41013 {
		t.Fatalf("done -> processing should be 41013, got %v", err)
	}

	if _, err := svc.Update(ctx, f.adminID, 999999, model.UpdateCSTicketRequest{
		Status: model.CSTicketStatusProcessing,
	}); mustBizCode(t, err) != 40401 {
		t.Fatalf("missing ticket should be 40401, got %v", err)
	}
	if _, err := svc.Update(ctx, f.adminID, ticketID, model.UpdateCSTicketRequest{
		Status: "bogus",
	}); mustBizCode(t, err) != 40001 {
		t.Fatalf("invalid status should be 40001, got %v", err)
	}
	if _, err := svc.Update(ctx, f.adminID, ticketID, model.UpdateCSTicketRequest{
		Status: model.CSTicketStatusDone, Remark: strings.Repeat("备", 501),
	}); mustBizCode(t, err) != 40001 {
		t.Fatalf("long remark should be 40001, got %v", err)
	}
}
