package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// refundFixture 在报名测试装配之上挂载退款工单服务。
type refundFixture struct {
	*regFixture
	svc     *RefundService
	adminID int64
	seq     int
}

func newRefundFixture(t *testing.T) *refundFixture {
	t.Helper()
	f := newRegFixture(t)
	var adminID int64
	if err := f.db.SQL().QueryRowContext(context.Background(),
		`SELECT id FROM admins WHERE username = 'admin'`).Scan(&adminID); err != nil {
		t.Fatalf("query admin id: %v", err)
	}
	return &refundFixture{regFixture: f, svc: NewRefundService(f.stores), adminID: adminID}
}

// paidOrder 创建报名中的赛事，并构造一条已支付订单，返回赛事/组别 ID 与订单号。
func (f *refundFixture) paidOrder(t *testing.T, tag string) (eventID, groupID int64, orderNo string) {
	t.Helper()
	eventID = mustCreateRegisteringEvent(t, f.regFixture, "退款赛事-"+tag, []model.AdminEventGroupInput{
		{Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Quota: 10},
	})
	groupID = mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)
	userID := mustInsertRegUser(t, f.db, "o-refund-"+tag)
	f.seq++
	playerNo := fmt.Sprintf("P%07d", f.seq) // players.player_no 固定 8 位字母数字
	orderNo = mustOrder(t, f.db, eventID, groupID, userID, playerNo, "ORD-"+tag, model.OrderStatusPaid)
	return eventID, groupID, orderNo
}

// setOrderStatus 直接改订单状态（测试构造退款中/已退款等前置状态）。
func (f *refundFixture) setOrderStatus(t *testing.T, orderNo, status string) {
	t.Helper()
	mustExecSQL(t, f.db, `UPDATE orders SET status = ? WHERE order_no = ?`, status, orderNo)
}

// setEventRegEnd 直接改赛事报名窗口截止时间（测试退款时间窗口边界）。
// 同时前移报名开始时间，满足 reg_start_at < reg_end_at 的时间线约束。
func (f *refundFixture) setEventRegEnd(t *testing.T, eventID int64, regEnd string) {
	t.Helper()
	mustExecSQL(t, f.db,
		`UPDATE events SET reg_start_at = '2019-12-01T00:00:00+08:00', reg_end_at = ? WHERE id = ?`,
		regEnd, eventID)
}

// countAdminLog 统计指定动作与目标上的审计日志条数。
func (f *refundFixture) countAdminLog(t *testing.T, action, targetType string, targetID int64) int {
	t.Helper()
	var n int
	if err := f.db.SQL().QueryRowContext(context.Background(),
		`SELECT COUNT(1) FROM admin_logs WHERE action = ? AND target_type = ? AND target_id = ?`,
		action, targetType, targetID).Scan(&n); err != nil {
		t.Fatalf("count admin log: %v", err)
	}
	return n
}

// createTicketReq 构造代录退款工单请求。
func createTicketReq(orderNo, reason string) model.CreateRefundTicketRequest {
	return model.CreateRefundTicketRequest{OrderNo: orderNo, Reason: reason}
}
