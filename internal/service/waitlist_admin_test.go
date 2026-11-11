package service

import (
	"context"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
)

// adminWaitlistFixture 创建一个满员赛事、两个候补记录，返回 fixture/赛事/组别/用户。
type adminWaitlistFixture struct {
	*waitlistFixture
	eventID  int64
	groupID  int64
	waitlist int64
	userID   int64
}

func newAdminWaitlistFixture(t *testing.T) *adminWaitlistFixture {
	t.Helper()
	f := newWaitlistFixture(t)
	eventID, groupID := f.createFullGroupEvent(t, "后台候补赛事")
	userID := mustInsertRegUser(t, f.db, "o-admin-wait-owner")
	resp, err := f.svc.Submit(context.Background(), userID, eventID,
		waitlistReq(groupID, "张小雨", validIDForBirth("20160307"), "13812345678"))
	if err != nil {
		t.Fatalf("submit waitlist: %v", err)
	}
	return &adminWaitlistFixture{waitlistFixture: f, eventID: eventID, groupID: groupID,
		waitlist: resp.WaitlistID, userID: userID}
}

// TestWaitlistAdminList 覆盖筛选、排序、脱敏字段与释放名额统计。
func TestWaitlistAdminList(t *testing.T) {
	a := newAdminWaitlistFixture(t)
	ctx := context.Background()
	userB := mustInsertRegUser(t, a.db, "o-admin-wait-b")
	userC := mustInsertRegUser(t, a.db, "o-admin-wait-c")

	// 第二古典舞候补（序号 2）。
	if _, err := a.svc.Submit(ctx, userB, a.eventID,
		waitlistReq(a.groupID, "李小雨", validIDForBirth("20170615"), "13912345678")); err != nil {
		t.Fatalf("submit B: %v", err)
	}
	// 另建民族民间舞组别并提交一条候补（用于 dance_type 筛选）。
	mustExecSQL(t, a.db, `INSERT INTO event_groups (event_id, level, dance_type, birth_year_from, birth_year_to, quota, taken)
		VALUES (?, 'U11', 'folk', 2016, 2018, 1, 1)`, a.eventID)
	folkGroupID := mustGroupID(t, a.db, a.eventID, model.GroupLevelU11, model.DanceTypeFolk)
	if _, err := a.svc.Submit(ctx, userC, a.eventID,
		waitlistReq(folkGroupID, "王小雨", validIDForBirth("20180615"), "13712345678")); err != nil {
		t.Fatalf("submit folk: %v", err)
	}

	resp, err := a.svc.AdminList(ctx, model.AdminWaitlistListQuery{
		EventID: a.eventID, Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("admin list: %v", err)
	}
	if resp.Total != 2 || len(resp.List) != 2 {
		t.Fatalf("admin list total = %d len = %d, want 2", resp.Total, len(resp.List))
	}
	if resp.List[0].PositionNo != 1 || resp.List[1].PositionNo != 2 {
		t.Fatalf("position order = %+v", resp.List)
	}
	first := resp.List[0]
	if first.Name != "张小雨" || first.Phone != "13812345678" || first.Level != model.GroupLevelU11 ||
		first.EventName != "后台候补赛事" || first.DanceType != model.DanceTypeClassical {
		t.Fatalf("admin item = %+v", first)
	}
	if first.IDCardMasked == nil || *first.IDCardMasked == "" {
		t.Fatalf("id_card_masked missing: %+v", first)
	}
	// 满员时释放名额为 0；调大名额后释放 1。
	if resp.ReleasedQuota != 0 {
		t.Fatalf("released quota while full = %d, want 0", resp.ReleasedQuota)
	}
	mustExecSQL(t, a.db, `UPDATE event_groups SET quota = quota + 1 WHERE id = ?`, a.groupID)
	resp, err = a.svc.AdminList(ctx, model.AdminWaitlistListQuery{
		EventID: a.eventID, Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("admin list after quota: %v", err)
	}
	if resp.ReleasedQuota != 1 {
		t.Fatalf("released quota = %d, want 1", resp.ReleasedQuota)
	}

	// status 筛选。
	if got, err := a.svc.AdminList(ctx, model.AdminWaitlistListQuery{
		EventID: a.eventID, Status: model.WaitlistStatusWaiting, Page: 1, PageSize: 20,
	}); err != nil || got.Total != 3 {
		t.Fatalf("status filter total = %v, %v, want 3", got, err)
	}

	// 非法筛选值 → 40001。
	_, err = a.svc.AdminList(ctx, model.AdminWaitlistListQuery{Status: "bogus"})
	assertBizCode(t, err, httpx.CodeBadRequest)
	_, err = a.svc.AdminList(ctx, model.AdminWaitlistListQuery{Level: "U99"})
	assertBizCode(t, err, httpx.CodeBadRequest)
	_, err = a.svc.AdminList(ctx, model.AdminWaitlistListQuery{DanceType: "ballet"})
	assertBizCode(t, err, httpx.CodeBadRequest)
}

// TestWaitlistAdminNotify 覆盖标记已通知、通知结果校验与审计日志。
func TestWaitlistAdminNotify(t *testing.T) {
	a := newAdminWaitlistFixture(t)
	ctx := context.Background()

	resp, err := a.svc.AdminNotify(ctx, 1, a.waitlist, model.WaitlistNoteConfirmedIntent)
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	if resp.Status != model.WaitlistStatusNotified {
		t.Fatalf("notify resp = %+v", resp)
	}
	var notifiedAt *string
	if err := a.db.SQL().QueryRowContext(ctx,
		`SELECT notified_at FROM waitlists WHERE id = ?`, a.waitlist).Scan(&notifiedAt); err != nil {
		t.Fatalf("query notified_at: %v", err)
	}
	if notifiedAt == nil || *notifiedAt == "" {
		t.Fatal("notified_at not set")
	}
	assertAdminLog(t, a, "admin.waitlist.notify")

	// 通知结果枚举非法 → 40001。
	_, err = a.svc.AdminNotify(ctx, 1, a.waitlist, "maybe")
	assertBizCode(t, err, httpx.CodeBadRequest)

	// 记录不存在 → 40401。
	_, err = a.svc.AdminNotify(ctx, 1, 9999, model.WaitlistNoteNoAnswer)
	assertBizCode(t, err, httpx.CodeNotFound)

	// 已移除 → 41013。
	mustExecSQL(t, a.db, `UPDATE waitlists SET status = 'removed' WHERE id = ?`, a.waitlist)
	_, err = a.svc.AdminNotify(ctx, 1, a.waitlist, model.WaitlistNoteCallbackLater)
	assertBizCode(t, err, httpx.CodeWaitlistStateDenied)
}

// TestWaitlistAdminRemove 覆盖移出队列、原因校验与终态不可重复操作。
func TestWaitlistAdminRemove(t *testing.T) {
	a := newAdminWaitlistFixture(t)
	ctx := context.Background()

	_, err := a.svc.AdminRemove(ctx, 1, a.waitlist, "  ")
	assertBizCode(t, err, httpx.CodeBadRequest)

	resp, err := a.svc.AdminRemove(ctx, 1, a.waitlist, "多次联系不上")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if resp.Status != model.WaitlistStatusRemoved {
		t.Fatalf("remove resp = %+v", resp)
	}
	var handledAt *string
	if err := a.db.SQL().QueryRowContext(ctx,
		`SELECT handled_at FROM waitlists WHERE id = ?`, a.waitlist).Scan(&handledAt); err != nil {
		t.Fatalf("query handled_at: %v", err)
	}
	if handledAt == nil || *handledAt == "" {
		t.Fatal("handled_at not set")
	}
	assertAdminLog(t, a, "admin.waitlist.remove")

	// 重复移出 → 41013。
	_, err = a.svc.AdminRemove(ctx, 1, a.waitlist, "再次移出")
	assertBizCode(t, err, httpx.CodeWaitlistStateDenied)
}

// TestWaitlistAdminConvert 覆盖手动关联已支付订单的校验与幂等。
func TestWaitlistAdminConvert(t *testing.T) {
	a := newAdminWaitlistFixture(t)
	ctx := context.Background()

	// 订单不存在 → 40001。
	_, err := a.svc.AdminConvert(ctx, 1, a.waitlist, "NO-SUCH-ORDER")
	assertBizCode(t, err, httpx.CodeBadRequest)

	// 未支付订单 → 40001。
	unpaid := mustOrder(t, a.db, a.eventID, a.groupID, a.userID, "PAY00001", "ORDER-UNPAID-1", model.OrderStatusPendingPayment)
	_, err = a.svc.AdminConvert(ctx, 1, a.waitlist, unpaid)
	assertBizCode(t, err, httpx.CodeBadRequest)

	// 非本人订单 → 40001。
	otherUser := mustInsertRegUser(t, a.db, "o-admin-wait-convert-other")
	otherOrder := mustOrder(t, a.db, a.eventID, a.groupID, otherUser, "PAY00002", "ORDER-OTHER-1", model.OrderStatusPaid)
	_, err = a.svc.AdminConvert(ctx, 1, a.waitlist, otherOrder)
	assertBizCode(t, err, httpx.CodeBadRequest)

	// 报名单元不一致 → 40001。
	mustExecSQL(t, a.db, `INSERT INTO event_groups (event_id, level, dance_type, birth_year_from, birth_year_to, quota, taken)
		VALUES (?, 'U13', 'classical', 2014, 2015, 1, 1)`, a.eventID)
	otherGroupID := mustGroupID(t, a.db, a.eventID, model.GroupLevelU13, model.DanceTypeClassical)
	wrongGroupOrder := mustOrder(t, a.db, a.eventID, otherGroupID, a.userID, "PAY00003", "ORDER-WRONG-GROUP-1", model.OrderStatusPaid)
	_, err = a.svc.AdminConvert(ctx, 1, a.waitlist, wrongGroupOrder)
	assertBizCode(t, err, httpx.CodeBadRequest)

	// 正常关联 → converted + handled_at + 审计日志。
	orderNo := mustOrder(t, a.db, a.eventID, a.groupID, a.userID, "PAY00004", "ORDER-OK-1", model.OrderStatusPaid)
	resp, err := a.svc.AdminConvert(ctx, 1, a.waitlist, orderNo)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if resp.Status != model.WaitlistStatusConverted {
		t.Fatalf("convert resp = %+v", resp)
	}
	assertWaitlistStatus(t, a.waitlistFixture, a.waitlist, model.WaitlistStatusConverted)
	assertAdminLog(t, a, "admin.waitlist.convert")

	// 重复确认幂等。
	resp, err = a.svc.AdminConvert(ctx, 1, a.waitlist, orderNo)
	if err != nil || resp.Status != model.WaitlistStatusConverted {
		t.Fatalf("repeat convert = %+v, %v", resp, err)
	}

	// 已移除记录 → 41013。
	fresh := newAdminWaitlistFixture(t)
	mustExecSQL(t, fresh.db, `UPDATE waitlists SET status = 'removed' WHERE id = ?`, fresh.waitlist)
	_, err = fresh.svc.AdminConvert(ctx, 1, fresh.waitlist, "ANY")
	assertBizCode(t, err, httpx.CodeWaitlistStateDenied)
}

// assertAdminLog 断言指定 action 的候补审计日志已落库。
func assertAdminLog(t *testing.T, a *adminWaitlistFixture, action string) {
	t.Helper()
	var n int
	if err := a.db.SQL().QueryRowContext(context.Background(),
		`SELECT COUNT(1) FROM admin_logs WHERE action = ? AND target_type = 'waitlist' AND target_id = ?`,
		action, a.waitlist).Scan(&n); err != nil {
		t.Fatalf("query admin log: %v", err)
	}
	if n != 1 {
		t.Fatalf("admin log %q count = %d, want 1", action, n)
	}
}
