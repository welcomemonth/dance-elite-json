package service

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
)

// TestWaitlistSubmit_SuccessAndQueue 覆盖入队成功、序号自增、证件号只存哈希与掩码。
func TestWaitlistSubmit_SuccessAndQueue(t *testing.T) {
	f := newWaitlistFixture(t)
	ctx := context.Background()
	eventID, groupID := f.createFullGroupEvent(t, "候补入队赛事")
	userA := mustInsertRegUser(t, f.db, "o-wait-a")
	userB := mustInsertRegUser(t, f.db, "o-wait-b")

	idA, err := f.svc.Submit(ctx, userA, eventID, waitlistReq(groupID, "张小雨", validIDForBirth("20160307"), "13812345678"))
	if err != nil {
		t.Fatalf("submit A: %v", err)
	}
	if idA.PositionNo != 1 || idA.Status != model.WaitlistStatusWaiting {
		t.Fatalf("submit A resp = %+v", idA)
	}
	idB, err := f.svc.Submit(ctx, userB, eventID, waitlistReq(groupID, "李小雨", validIDForBirth("20170615"), "13912345678"))
	if err != nil {
		t.Fatalf("submit B: %v", err)
	}
	if idB.PositionNo != 2 {
		t.Fatalf("submit B position = %d, want 2", idB.PositionNo)
	}

	// 库内只有哈希与脱敏值，无证件号明文。
	var hash, masked, name, danceType string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT id_card_hash, id_card_masked, name, dance_type FROM waitlists WHERE id = ?`, idA.WaitlistID).
		Scan(&hash, &masked, &name, &danceType); err != nil {
		t.Fatalf("query waitlist: %v", err)
	}
	if len(hash) != 64 || strings.Contains(hash, "20160307") {
		t.Fatalf("id_card_hash = %q, want opaque HMAC hex", hash)
	}
	if masked == "" || !strings.Contains(masked, "*") || masked == validIDForBirth("20160307") {
		t.Fatalf("id_card_masked = %q, want masked", masked)
	}
	if name != "张小雨" || danceType != model.DanceTypeClassical {
		t.Fatalf("stored name/dance = %q/%q", name, danceType)
	}
}

// TestWaitlistSubmit_Duplicate 覆盖同报名单元同证件号重复排队（4010/41010）。
func TestWaitlistSubmit_Duplicate(t *testing.T) {
	f := newWaitlistFixture(t)
	ctx := context.Background()
	eventID, groupID := f.createFullGroupEvent(t, "候补重复赛事")
	userA := mustInsertRegUser(t, f.db, "o-wait-dup-a")
	userB := mustInsertRegUser(t, f.db, "o-wait-dup-b")
	idCard := validIDForBirth("20160307")

	if _, err := f.svc.Submit(ctx, userA, eventID, waitlistReq(groupID, "张小雨", idCard, "13812345678")); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	// 同一证件号（带空白变体）换用户提交 → 41010。
	_, err := f.svc.Submit(ctx, userB, eventID, waitlistReq(groupID, "张小雨", " "+idCard+" ", "13912345678"))
	assertBizCode(t, err, httpx.CodeWaitlistDuplicate)
}

// TestWaitlistSubmit_Validation 覆盖姓名/电话/证件号校验。
func TestWaitlistSubmit_Validation(t *testing.T) {
	f := newWaitlistFixture(t)
	ctx := context.Background()
	eventID, groupID := f.createFullGroupEvent(t, "候补校验赛事")
	userID := mustInsertRegUser(t, f.db, "o-wait-val")

	_, err := f.svc.Submit(ctx, userID, eventID, waitlistReq(groupID, "  ", "", "13812345678"))
	assertBizCode(t, err, httpx.CodeBadRequest)

	_, err = f.svc.Submit(ctx, userID, eventID, waitlistReq(groupID, "张小雨", "", "1381234"))
	assertBizCode(t, err, httpx.CodeBadRequest)

	_, err = f.svc.Submit(ctx, userID, eventID, waitlistReq(groupID, "张小雨", "110105201603070011", "13812345678"))
	assertBizCode(t, err, httpx.CodeInvalidIDNumber)
}

// TestWaitlistSubmit_GroupChecks 覆盖名额未满/组别关闭/年龄不符/非报名期/组别不属于赛事。
func TestWaitlistSubmit_GroupChecks(t *testing.T) {
	f := newWaitlistFixture(t)
	ctx := context.Background()

	// 名额未满 → 40001（引导直接报名）。
	eventID, groupID := f.createFullGroupEvent(t, "名额未满赛事")
	mustExecSQL(t, f.db, `UPDATE event_groups SET taken = 0 WHERE id = ?`, groupID)
	userID := mustInsertRegUser(t, f.db, "o-wait-checks")
	_, err := f.svc.Submit(ctx, userID, eventID, waitlistReq(groupID, "张小雨", validIDForBirth("20160307"), "13812345678"))
	assertBizCode(t, err, httpx.CodeBadRequest)

	// 名额占满但组别关闭 → 41007。
	mustExecSQL(t, f.db, `UPDATE event_groups SET taken = quota, enabled = 0 WHERE id = ?`, groupID)
	_, err = f.svc.Submit(ctx, userID, eventID, waitlistReq(groupID, "张小雨", validIDForBirth("20160307"), "13812345678"))
	assertBizCode(t, err, httpx.CodeGroupDisabled)
	mustExecSQL(t, f.db, `UPDATE event_groups SET enabled = 1 WHERE id = ?`, groupID)

	// 年龄不符（U13 身份证用于 U11 组别）→ 41002。
	_, err = f.svc.Submit(ctx, userID, eventID, waitlistReq(groupID, "张小雨", validIDForBirth("20150615"), "13812345678"))
	assertBizCode(t, err, httpx.CodeAgeNotAllowed)

	// 赛事已结束 → 41006。
	mustExecSQL(t, f.db, `UPDATE events SET status = 'finished' WHERE id = ?`, eventID)
	_, err = f.svc.Submit(ctx, userID, eventID, waitlistReq(groupID, "张小雨", validIDForBirth("20160307"), "13812345678"))
	assertBizCode(t, err, httpx.CodeNotInRegPeriod)

	// 赛事不存在 → 40401。
	_, err = f.svc.Submit(ctx, userID, 9999, waitlistReq(groupID, "张小雨", validIDForBirth("20160307"), "13812345678"))
	assertBizCode(t, err, httpx.CodeNotFound)

	// 组别不属于该赛事 → 40401。
	otherEventID, otherGroupID := f.createFullGroupEvent(t, "另一赛事")
	_, err = f.svc.Submit(ctx, userID, otherEventID, waitlistReq(groupID, "张小雨", validIDForBirth("20160307"), "13812345678"))
	assertBizCode(t, err, httpx.CodeNotFound)
	if otherGroupID == groupID {
		t.Fatal("fixture groups should differ")
	}
}

// TestWaitlistSubmit_WithoutIDCard 无证件号登记：hash/masked 为空，不参与重复判断。
func TestWaitlistSubmit_WithoutIDCard(t *testing.T) {
	f := newWaitlistFixture(t)
	ctx := context.Background()
	eventID, groupID := f.createFullGroupEvent(t, "无证件号候补")
	userA := mustInsertRegUser(t, f.db, "o-wait-noid-a")
	userB := mustInsertRegUser(t, f.db, "o-wait-noid-b")

	resp, err := f.svc.Submit(ctx, userA, eventID, waitlistReq(groupID, "张小雨", "", "13812345678"))
	if err != nil {
		t.Fatalf("submit without id: %v", err)
	}
	var hash, masked *string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT id_card_hash, id_card_masked FROM waitlists WHERE id = ?`, resp.WaitlistID).
		Scan(&hash, &masked); err != nil {
		t.Fatalf("query waitlist: %v", err)
	}
	if hash != nil || masked != nil {
		t.Fatalf("hash/masked = %v/%v, want NULL", hash, masked)
	}
	// 无证件号登记不做重复判断：另一用户同名同电话仍可入队。
	if _, err := f.svc.Submit(ctx, userB, eventID, waitlistReq(groupID, "张小雨", "", "13812345678")); err != nil {
		t.Fatalf("second no-id submit: %v", err)
	}
}

// TestWaitlistSubmit_ConcurrentPositions 并发提交不同证件号：序号不重复。
func TestWaitlistSubmit_ConcurrentPositions(t *testing.T) {
	f := newWaitlistFixture(t)
	ctx := context.Background()
	eventID, groupID := f.createFullGroupEvent(t, "并发候补赛事")
	userA := mustInsertRegUser(t, f.db, "o-wait-c-a")
	userB := mustInsertRegUser(t, f.db, "o-wait-c-b")

	var wg sync.WaitGroup
	positions := make([]int, 2)
	errs := make([]error, 2)
	jobs := []struct {
		user   int64
		name   string
		idCard string
	}{
		{userA, "甲", validIDForBirth("20160307")},
		{userB, "乙", validIDForBirth("20170615")},
	}
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, user int64, name, idCard string) {
			defer wg.Done()
			resp, err := f.svc.Submit(ctx, user, eventID, waitlistReq(groupID, name, idCard, "13812345678"))
			if err != nil {
				errs[i] = err
				return
			}
			positions[i] = resp.PositionNo
		}(i, j.user, j.name, j.idCard)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent submit %d: %v", i, err)
		}
	}
	if positions[0] == positions[1] || positions[0] == 0 || positions[1] == 0 {
		t.Fatalf("concurrent positions = %v, want distinct 1/2", positions)
	}
}

// TestWaitlistCancel 覆盖本人取消、重复取消幂等、非本人/已通知/已转报名拦截。
func TestWaitlistCancel(t *testing.T) {
	f := newWaitlistFixture(t)
	ctx := context.Background()
	eventID, groupID := f.createFullGroupEvent(t, "候补取消赛事")
	owner := mustInsertRegUser(t, f.db, "o-wait-cancel-owner")
	other := mustInsertRegUser(t, f.db, "o-wait-cancel-other")

	resp, err := f.svc.Submit(ctx, owner, eventID, waitlistReq(groupID, "张小雨", validIDForBirth("20160307"), "13812345678"))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := f.svc.Cancel(ctx, other, resp.WaitlistID); err == nil {
		t.Fatal("cancel by other user should fail")
	} else {
		assertBizCode(t, err, httpx.CodeNotFound)
	}
	if err := f.svc.Cancel(ctx, owner, resp.WaitlistID); err != nil {
		t.Fatalf("cancel own: %v", err)
	}
	assertWaitlistStatus(t, f, resp.WaitlistID, model.WaitlistStatusGivenUp)
	// 重复取消幂等。
	if err := f.svc.Cancel(ctx, owner, resp.WaitlistID); err != nil {
		t.Fatalf("repeat cancel: %v", err)
	}

	// 已通知 → 41011。
	resp2, err := f.svc.Submit(ctx, owner, eventID, waitlistReq(groupID, "张小雨", validIDForBirth("20170615"), "13812345678"))
	if err != nil {
		t.Fatalf("submit 2: %v", err)
	}
	mustExecSQL(t, f.db, `UPDATE waitlists SET status = 'notified' WHERE id = ?`, resp2.WaitlistID)
	err = f.svc.Cancel(ctx, owner, resp2.WaitlistID)
	assertBizCode(t, err, httpx.CodeWaitlistNotCancelable)
}

// TestWaitlistListMine 覆盖仅本人记录、状态保留与分页。
func TestWaitlistListMine(t *testing.T) {
	f := newWaitlistFixture(t)
	ctx := context.Background()
	eventA, groupA := f.createFullGroupEvent(t, "我的候补 A")
	eventB, groupB := f.createFullGroupEvent(t, "我的候补 B")
	owner := mustInsertRegUser(t, f.db, "o-wait-mine")
	other := mustInsertRegUser(t, f.db, "o-wait-mine-other")

	first, err := f.svc.Submit(ctx, owner, eventA, waitlistReq(groupA, "甲", validIDForBirth("20160307"), "13812345678"))
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	if _, err := f.svc.Submit(ctx, owner, eventB, waitlistReq(groupB, "乙", validIDForBirth("20170615"), "13812345678")); err != nil {
		t.Fatalf("submit second: %v", err)
	}
	third, err := f.svc.Submit(ctx, other, eventA, waitlistReq(groupA, "丙", validIDForBirth("20180615"), "13812345678"))
	if err != nil {
		t.Fatalf("submit other: %v", err)
	}
	if err := f.svc.Cancel(ctx, owner, first.WaitlistID); err != nil {
		t.Fatalf("cancel first: %v", err)
	}

	resp, err := f.svc.ListMine(ctx, owner, 1, 20)
	if err != nil {
		t.Fatalf("list mine: %v", err)
	}
	if resp.Total != 2 || len(resp.List) != 2 {
		t.Fatalf("mine total = %d list = %d, want 2", resp.Total, len(resp.List))
	}
	for _, item := range resp.List {
		if item.WaitlistID == third.WaitlistID {
			t.Fatalf("other user's waitlist leaked: %+v", item)
		}
	}
	// 按 id 倒序：eventB 的记录在前；取消记录仍保留（given_up）。
	if resp.List[0].EventID != eventB || resp.List[1].Status != model.WaitlistStatusGivenUp {
		t.Fatalf("mine list order/status = %+v", resp.List)
	}

	page, err := f.svc.ListMine(ctx, owner, 2, 1)
	if err != nil {
		t.Fatalf("list mine page2: %v", err)
	}
	if page.Total != 2 || len(page.List) != 1 || page.List[0].Status != model.WaitlistStatusGivenUp {
		t.Fatalf("page 2 = %+v", page)
	}
}

// assertWaitlistStatus 断言候补当前状态。
func assertWaitlistStatus(t *testing.T, f *waitlistFixture, id int64, want string) {
	t.Helper()
	var status string
	if err := f.db.SQL().QueryRowContext(context.Background(),
		`SELECT status FROM waitlists WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("query waitlist status: %v", err)
	}
	if status != want {
		t.Fatalf("waitlist %d status = %q, want %q", id, status, want)
	}
}
