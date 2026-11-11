package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// idCardGenWeights 与 utils.idCardWeights 一致（GB 11643-1999 加权因子），
// 测试内联一份以便按出生日期生成合法身份证。
var idCardGenWeights = [...]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}

const idCardGenCheckCodes = "10X98765432"

// validIDForBirth 生成指定出生日期（YYYYMMDD）的合法身份证号（测试辅助）。
func validIDForBirth(birth string) string {
	prefix := "110105" + birth + "001"
	sum := 0
	for i := 0; i < 17; i++ {
		sum += int(prefix[i]-'0') * idCardGenWeights[i]
	}
	return prefix + string(idCardGenCheckCodes[sum%11])
}

// regFixture 是报名下单测试的装配：内存库 + 已发布赛事服务 + 报名服务。
type regFixture struct {
	stores   *store.Store
	db       *store.DB
	eventSvc *EventService
	regSvc   *RegistrationService
	seasonID int64
}

func newRegFixture(t *testing.T) *regFixture {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db.SQL()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Seed(context.Background(), store.SeedOptions{
		AdminUsername: "admin", AdminPassword: "Init@12345",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stores := store.New(db)
	seasonSvc := NewSeasonService(stores)
	eventSvc := NewEventService(stores)
	players := NewPlayerService(stores, "test-hmac-key")
	season := mustActiveSeason(t, seasonSvc)
	return &regFixture{
		stores:   stores,
		db:       db,
		eventSvc: eventSvc,
		regSvc:   NewRegistrationService(stores, players),
		seasonID: season.ID,
	}
}

// isoOffset 返回 now 偏移 hours 小时后的统一 ISO 时间串（+08:00）。
func isoOffset(hours time.Duration) string {
	return time.Now().Add(hours * time.Hour).In(utils.ChinaLoc).Format("2006-01-02T15:04:05-07:00")
}

// registeringEventReq 构造报名窗口为 [now-1h, now+24h] 的发布赛事请求。
func registeringEventReq(seasonID int64, name string, groups []model.AdminEventGroupInput) model.AdminCreateEventRequest {
	fee := int64(98000)
	return model.AdminCreateEventRequest{
		Name:      name,
		SeasonID:  seasonID,
		Level:     model.EventLevelGradeA,
		City:      "上海",
		Venue:     "上海国际舞蹈中心",
		EventDate: "2027-03-15",
		Fee:       &fee,
		Timeline: model.EventTimeline{
			RegStartAt:      isoOffset(-1),
			RegEndAt:        isoOffset(24),
			LotteryAt:       isoOffset(48),
			MaterialCheckAt: isoOffset(72),
			StartAt:         isoOffset(96),
		},
		Groups:  groups,
		Publish: true,
	}
}

var defaultRegGroups = []model.AdminEventGroupInput{
	{Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Quota: 40},
	{Level: model.GroupLevelU11, DanceType: model.DanceTypeFolk, Quota: 40},
	{Level: model.GroupLevelU13, DanceType: model.DanceTypeClassical, Quota: 40},
}

// mustCreateRegisteringEvent 创建并发布赛事，返回赛事 ID。
func mustCreateRegisteringEvent(t *testing.T, f *regFixture, name string, groups []model.AdminEventGroupInput) int64 {
	t.Helper()
	resp, err := f.eventSvc.Create(context.Background(), 1, registeringEventReq(f.seasonID, name, groups))
	if err != nil {
		t.Fatalf("create event %s: %v", name, err)
	}
	return resp.EventID
}

// mustInsertRegUser 通过原始 SQL 插入小程序用户，返回 user_id。
func mustInsertRegUser(t *testing.T, db *store.DB, openid string) int64 {
	t.Helper()
	res, err := db.SQL().ExecContext(context.Background(), `INSERT INTO users (openid) VALUES (?)`, openid)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("user id: %v", err)
	}
	return id
}

// submitReq 构造提交报名请求。
func submitReq(idCard, danceType, key string) model.SubmitRegistrationRequest {
	return model.SubmitRegistrationRequest{
		PlayerName:     "王小舞",
		IDCardNo:       idCard,
		Institution:    "小天鹅舞蹈工作室",
		DanceType:      danceType,
		IdempotencyKey: key,
	}
}

// groupTaken 读取报名单元的 taken 计数（对账断言用）。
func groupTaken(t *testing.T, db *store.DB, groupID int64) int {
	t.Helper()
	var taken int
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT taken FROM event_groups WHERE id = ?`, groupID).Scan(&taken); err != nil {
		t.Fatalf("query taken of group %d: %v", groupID, err)
	}
	return taken
}

// mustGroupID 取赛事某级别×舞种的组别 ID。
func mustGroupID(t *testing.T, db *store.DB, eventID int64, level, danceType string) int64 {
	t.Helper()
	var id int64
	if err := db.SQL().QueryRowContext(context.Background(),
		`SELECT id FROM event_groups WHERE event_id = ? AND level = ? AND dance_type = ?`,
		eventID, level, danceType).Scan(&id); err != nil {
		t.Fatalf("query group %s/%s of event %d: %v", level, danceType, eventID, err)
	}
	return id
}

func TestSubmit_Success(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	eventID := mustCreateRegisteringEvent(t, f, "报名中赛事", defaultRegGroups)
	userID := mustInsertRegUser(t, f.db, "o-submit-1")
	groupID := mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)

	resp, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-success-0001"))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if len(resp.OrderNo) != 22 || resp.RegistrationID <= 0 {
		t.Fatalf("resp = %+v, want 22-digit order_no and registration id", resp)
	}
	if resp.Amount != 98000 || resp.Level != model.GroupLevelU11 || resp.DanceType != model.DanceTypeClassical {
		t.Fatalf("resp = %+v", resp)
	}

	// 落库断言：registration + order 均为 pending_payment，taken +1。
	var regStatus, orderStatus, requestID string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT status FROM registrations WHERE id = ?`, resp.RegistrationID).Scan(&regStatus); err != nil {
		t.Fatalf("query registration: %v", err)
	}
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT status, request_id FROM orders WHERE order_no = ?`, resp.OrderNo).Scan(&orderStatus, &requestID); err != nil {
		t.Fatalf("query order: %v", err)
	}
	if regStatus != model.RegistrationStatusPendingPayment || orderStatus != model.OrderStatusPendingPayment {
		t.Fatalf("status: reg=%q order=%q, want pending_payment", regStatus, orderStatus)
	}
	if requestID != "key-success-0001" {
		t.Fatalf("request_id = %q", requestID)
	}
	if took := groupTaken(t, f.db, groupID); took != 1 {
		t.Fatalf("taken = %d, want 1", took)
	}
	// 资料截止 = 赛事核对音频时间。
	var materialCheck string
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT material_check_at FROM events WHERE id = ?`, eventID).Scan(&materialCheck); err != nil {
		t.Fatalf("query event: %v", err)
	}
	if resp.MaterialDeadline == nil || *resp.MaterialDeadline != materialCheck {
		t.Fatalf("material_deadline = %v, want %q", resp.MaterialDeadline, materialCheck)
	}
}

func TestSubmit_EventStateErrors(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	req := submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-event-state")

	// 不存在 → 40401。
	if _, err := f.regSvc.Submit(ctx, 9999, 9999, req); err == nil || bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("missing event err = %v, want 40401", err)
	}

	// 草稿（未发布）→ 40401。
	draft := registeringEventReq(f.seasonID, "草稿赛事", defaultRegGroups)
	draft.Publish = false
	draftResp, err := f.eventSvc.Create(ctx, 1, draft)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	userID := mustInsertRegUser(t, f.db, "o-event-state")
	if _, err := f.regSvc.Submit(ctx, userID, draftResp.EventID, req); err == nil || bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("draft event err = %v, want 40401", err)
	}

	// 未开始报名 → 41006。
	upcoming := registeringEventReq(f.seasonID, "未开始赛事", defaultRegGroups)
	upcoming.Timeline.RegStartAt = isoOffset(24)
	upcoming.Timeline.RegEndAt = isoOffset(48)
	upcoming.Timeline.LotteryAt = isoOffset(72)
	upcoming.Timeline.MaterialCheckAt = isoOffset(96)
	upcoming.Timeline.StartAt = isoOffset(120)
	upcomingResp, err := f.eventSvc.Create(ctx, 1, upcoming)
	if err != nil {
		t.Fatalf("create upcoming: %v", err)
	}
	if _, err := f.regSvc.Submit(ctx, userID, upcomingResp.EventID, req); err == nil || bizCode(t, err) != httpx.CodeNotInRegPeriod {
		t.Fatalf("upcoming event err = %v, want 41006", err)
	}

	// 报名已截止 → 41006。
	closed := registeringEventReq(f.seasonID, "已截止赛事", defaultRegGroups)
	closed.Timeline.RegStartAt = isoOffset(-2)
	closed.Timeline.RegEndAt = isoOffset(-1)
	closed.Timeline.LotteryAt = isoOffset(1)
	closedResp, err := f.eventSvc.Create(ctx, 1, closed)
	if err != nil {
		t.Fatalf("create closed: %v", err)
	}
	if _, err := f.regSvc.Submit(ctx, userID, closedResp.EventID, req); err == nil || bizCode(t, err) != httpx.CodeNotInRegPeriod {
		t.Fatalf("closed event err = %v, want 41006", err)
	}

	// 下线 → 40401。
	eventID := mustCreateRegisteringEvent(t, f, "将下线赛事", defaultRegGroups)
	if _, err := f.eventSvc.Offline(ctx, 1, eventID); err != nil {
		t.Fatalf("offline: %v", err)
	}
	if _, err := f.regSvc.Submit(ctx, userID, eventID, req); err == nil || bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("offline event err = %v, want 40401", err)
	}
}

func TestSubmit_ParamErrors(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	eventID := mustCreateRegisteringEvent(t, f, "参数校验赛事", defaultRegGroups)
	userID := mustInsertRegUser(t, f.db, "o-param")

	cases := []struct {
		name string
		req  model.SubmitRegistrationRequest
		want int
	}{
		{"空姓名", model.SubmitRegistrationRequest{PlayerName: "  ", IDCardNo: validIDCardMale2016, DanceType: model.DanceTypeClassical, IdempotencyKey: "key-param-0001"}, httpx.CodeBadRequest},
		{"舞种非法", submitReq(validIDCardMale2016, "ballet", "key-param-0002"), httpx.CodeBadRequest},
		{"身份证非法", submitReq("123", model.DanceTypeClassical, "key-param-0003"), httpx.CodeInvalidIDNumber},
		{"幂等键过短", submitReq(validIDCardMale2016, model.DanceTypeClassical, "short"), httpx.CodeBadRequest},
		{"幂等键含空白", submitReq(validIDCardMale2016, model.DanceTypeClassical, "key with space"), httpx.CodeBadRequest},
	}
	for _, tc := range cases {
		if _, err := f.regSvc.Submit(ctx, userID, eventID, tc.req); err == nil || bizCode(t, err) != tc.want {
			t.Fatalf("%s: err = %v, want code %d", tc.name, err, tc.want)
		}
	}
}

func TestSubmit_AgeNotAllowed(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	eventID := mustCreateRegisteringEvent(t, f, "年龄校验赛事", defaultRegGroups)
	userID := mustInsertRegUser(t, f.db, "o-age")

	// 2010 年出生：低于全部组别下限 → 41002，消息附出生年份与组别区间。
	_, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(validIDForBirth("20100307"), model.DanceTypeClassical, "key-age-000001"))
	if err == nil {
		t.Fatal("expected 41002")
	}
	biz := bizErr(t, err)
	if biz.Code != httpx.CodeAgeNotAllowed {
		t.Fatalf("err = %v, want 41002", err)
	}
	for _, want := range []string{"2010", "U11", "2016-2018"} {
		if !strings.Contains(biz.Message, want) {
			t.Fatalf("message %q should contain %q", biz.Message, want)
		}
	}
}

// bizErr 断言 err 为 BizError 并返回（非 BizError 直接失败）。
func bizErr(t *testing.T, err error) *httpx.BizError {
	t.Helper()
	var biz *httpx.BizError
	if !errors.As(err, &biz) {
		t.Fatalf("err = %v, want BizError", err)
	}
	return biz
}

func TestSubmit_GroupDisabled(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	eventID := mustCreateRegisteringEvent(t, f, "关闭组别赛事", defaultRegGroups)
	userID := mustInsertRegUser(t, f.db, "o-disabled")
	groupID := mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)

	if _, err := f.eventSvc.SetGroupEnabled(ctx, 1, eventID, groupID, false); err != nil {
		t.Fatalf("disable group: %v", err)
	}
	_, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-disabled-001"))
	if err == nil || bizCode(t, err) != httpx.CodeGroupDisabled {
		t.Fatalf("err = %v, want 41007", err)
	}
	if took := groupTaken(t, f.db, groupID); took != 0 {
		t.Fatalf("taken = %d, want 0（关闭组别不占名额）", took)
	}
}

func TestSubmit_IdempotentRetry(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	eventID := mustCreateRegisteringEvent(t, f, "幂等赛事", defaultRegGroups)
	userID := mustInsertRegUser(t, f.db, "o-idem")

	first, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-idem-000001"))
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	second, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-idem-000001"))
	if err != nil {
		t.Fatalf("retry submit: %v", err)
	}
	if first.OrderNo != second.OrderNo || first.RegistrationID != second.RegistrationID {
		t.Fatalf("idempotent retry returned different order: %+v vs %+v", first, second)
	}
	var n int
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(1) FROM orders WHERE user_id = ?`, userID).Scan(&n); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if n != 1 {
		t.Fatalf("orders = %d, want 1", n)
	}
}

func TestSubmit_DuplicateSameUnit(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	eventID := mustCreateRegisteringEvent(t, f, "判重赛事", defaultRegGroups)
	userID := mustInsertRegUser(t, f.db, "o-dup")
	groupID := mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)

	if _, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-dup-000001")); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	// 同证件同舞种、换新幂等键 → 41004。
	_, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-dup-000002"))
	if err == nil || bizCode(t, err) != httpx.CodeDanceTypeDuplicate {
		t.Fatalf("err = %v, want 41004", err)
	}
	// 跨用户同证件（家长两个微信号）同样被拦 → 同一 player 档案。
	user2 := mustInsertRegUser(t, f.db, "o-dup-2")
	_, err = f.regSvc.Submit(ctx, user2, eventID, submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-dup-000003"))
	if err == nil || bizCode(t, err) != httpx.CodeDanceTypeDuplicate {
		t.Fatalf("cross-user duplicate err = %v, want 41004", err)
	}
	if took := groupTaken(t, f.db, groupID); took != 1 {
		t.Fatalf("taken = %d, want 1（重复提交不占名额）", took)
	}

	// 同一组别另一舞种是合法报名（可报多个舞种）→ 成功。
	if _, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(validIDCardMale2016, model.DanceTypeFolk, "key-dup-000004")); err != nil {
		t.Fatalf("second dance type should be allowed: %v", err)
	}
}

func TestSubmit_DifferentLevelBlocked(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	eventID := mustCreateRegisteringEvent(t, f, "一人一组别赛事", defaultRegGroups)
	userID := mustInsertRegUser(t, f.db, "o-level")
	u13GroupID := mustGroupID(t, f.db, eventID, model.GroupLevelU13, model.DanceTypeClassical)

	// 先成功报名 U11 古典舞（2016 年出生）。
	if _, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-level-000001")); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	// 人为制造"档案被录入到 U13"的在途报名（T16 人工录入边界）：同 player 挂 U13 组别。
	var playerID int64
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT id FROM players WHERE name = ?`, "王小舞").Scan(&playerID); err != nil {
		t.Fatalf("query player: %v", err)
	}
	if _, err := f.db.SQL().ExecContext(ctx,
		`INSERT INTO registrations (event_id, group_id, player_id, user_id, dance_type, status)
		 VALUES (?, ?, ?, ?, 'classical', 'active')`, eventID, u13GroupID, playerID, userID); err != nil {
		t.Fatalf("insert u13 in-flight registration: %v", err)
	}

	// 再次提交（任意舞种幂等键）→ 41003 一人一组别，消息附已有报名。
	_, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(validIDCardMale2016, model.DanceTypeFolk, "key-level-000002"))
	biz := bizErr(t, err)
	if biz.Code != httpx.CodeLevelAlreadyTaken {
		t.Fatalf("err = %v, want 41003", err)
	}
	for _, want := range []string{"U13", "一人只能报名一个组别"} {
		if !strings.Contains(biz.Message, want) {
			t.Fatalf("message %q should contain %q", biz.Message, want)
		}
	}
}

func TestSubmit_QuotaFull(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	groups := []model.AdminEventGroupInput{
		{Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Quota: 1},
	}
	eventID := mustCreateRegisteringEvent(t, f, "名额有限赛事", groups)
	userA := mustInsertRegUser(t, f.db, "o-quota-a")
	userB := mustInsertRegUser(t, f.db, "o-quota-b")
	groupID := mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)

	if _, err := f.regSvc.Submit(ctx, userA, eventID, submitReq(validIDForBirth("20160307"), model.DanceTypeClassical, "key-quota-00001")); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	// 第二个选手（2017 年出生，同属 U11）抢最后一个名额 → 41005，data 附组别信息。
	_, err := f.regSvc.Submit(ctx, userB, eventID, submitReq(validIDForBirth("20170615"), model.DanceTypeClassical, "key-quota-00002"))
	if err == nil {
		t.Fatal("expected 41005")
	}
	biz := err.(*httpx.BizError)
	if biz.Code != httpx.CodeQuotaFull {
		t.Fatalf("err = %v, want 41005", err)
	}
	data, ok := biz.Data.(model.QuotaFullData)
	if !ok || data.GroupID != groupID || data.Level != model.GroupLevelU11 || data.DanceType != model.DanceTypeClassical {
		t.Fatalf("quota full data = %+v", biz.Data)
	}
	if took := groupTaken(t, f.db, groupID); took != 1 {
		t.Fatalf("taken = %d, want 1（满员后不再增长）", took)
	}
}

// TestSubmit_ConcurrencySamePlayer 同人同时双请求：只有一个成功（另一个 41004），
// 且只产生一个订单、占用一个名额。
func TestSubmit_ConcurrencySamePlayer(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	eventID := mustCreateRegisteringEvent(t, f, "并发同人赛事", defaultRegGroups)
	userA := mustInsertRegUser(t, f.db, "o-conc-a")
	userB := mustInsertRegUser(t, f.db, "o-conc-b")
	groupID := mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)

	type result struct {
		resp *model.SubmitRegistrationResponse
		code int
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, u := range []struct {
		userID int64
		key    string
	}{{userA, "key-conc-p-0001"}, {userB, "key-conc-p-0002"}} {
		wg.Add(1)
		go func(userID int64, key string) {
			defer wg.Done()
			resp, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(validIDCardMale2016, model.DanceTypeClassical, key))
			if err != nil {
				results <- result{code: bizCode(t, err)}
				return
			}
			results <- result{resp: resp}
		}(u.userID, u.key)
	}
	wg.Wait()
	close(results)

	success, dup := 0, 0
	for r := range results {
		if r.resp != nil {
			success++
		} else if r.code == httpx.CodeDanceTypeDuplicate {
			dup++
		} else {
			t.Fatalf("unexpected code %d", r.code)
		}
	}
	if success != 1 || dup != 1 {
		t.Fatalf("success=%d dup=%d, want 1/1", success, dup)
	}
	if took := groupTaken(t, f.db, groupID); took != 1 {
		t.Fatalf("taken = %d, want 1", took)
	}
}

// TestSubmit_ConcurrencyLastSlot 最后 1 个名额两并发请求只有一个抢到（另一个 41005）。
func TestSubmit_ConcurrencyLastSlot(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	groups := []model.AdminEventGroupInput{
		{Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Quota: 1},
	}
	eventID := mustCreateRegisteringEvent(t, f, "并发抢名额赛事", groups)
	userA := mustInsertRegUser(t, f.db, "o-slot-a")
	userB := mustInsertRegUser(t, f.db, "o-slot-b")
	groupID := mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)

	type result struct {
		resp *model.SubmitRegistrationResponse
		code int
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	players := []struct {
		userID int64
		idCard string
		key    string
	}{{userA, validIDForBirth("20160307"), "key-slot-000001"}, {userB, validIDForBirth("20170615"), "key-slot-000002"}}
	for _, p := range players {
		wg.Add(1)
		go func(userID int64, idCard, key string) {
			defer wg.Done()
			resp, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(idCard, model.DanceTypeClassical, key))
			if err != nil {
				results <- result{code: bizCode(t, err)}
				return
			}
			results <- result{resp: resp}
		}(p.userID, p.idCard, p.key)
	}
	wg.Wait()
	close(results)

	success, full := 0, 0
	for r := range results {
		if r.resp != nil {
			success++
		} else if r.code == httpx.CodeQuotaFull {
			full++
		} else {
			t.Fatalf("unexpected code %d", r.code)
		}
	}
	if success != 1 || full != 1 {
		t.Fatalf("success=%d full=%d, want 1/1（原子抢名额应只有一个成功）", success, full)
	}
	if took := groupTaken(t, f.db, groupID); took != 1 {
		t.Fatalf("taken = %d, want 1（不得超卖）", took)
	}
}

// TestSubmit_ConcurrencySameRequestID 同幂等键并发重试只产生一个订单。
func TestSubmit_ConcurrencySameRequestID(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	eventID := mustCreateRegisteringEvent(t, f, "并发幂等赛事", defaultRegGroups)
	userID := mustInsertRegUser(t, f.db, "o-conc-idem")

	type result struct {
		resp *model.SubmitRegistrationResponse
		err  error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(validIDCardMale2016, model.DanceTypeClassical, "key-conc-idem-01"))
			results <- result{resp: resp, err: err}
		}()
	}
	wg.Wait()
	close(results)

	var orderNos []string
	for r := range results {
		if r.err != nil {
			t.Fatalf("same-request-id concurrency should not fail: %v", r.err)
		}
		orderNos = append(orderNos, r.resp.OrderNo)
	}
	if orderNos[0] != orderNos[1] {
		t.Fatalf("concurrent same-key returned different orders: %v", orderNos)
	}
	var n int
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(1) FROM orders WHERE user_id = ?`, userID).Scan(&n); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if n != 1 {
		t.Fatalf("orders = %d, want 1", n)
	}
}

// TestSubmit_TakenConsistency taken 计数与进行中报名数对账（取消释放名额由 T12 回归）。
func TestSubmit_TakenConsistency(t *testing.T) {
	f := newRegFixture(t)
	ctx := context.Background()
	eventID := mustCreateRegisteringEvent(t, f, "对账赛事", defaultRegGroups)
	groupID := mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)

	players := []string{
		validIDForBirth("20160101"),
		validIDForBirth("20170202"),
		validIDForBirth("20180303"),
	}
	for i, idCard := range players {
		userID := mustInsertRegUser(t, f.db, fmt.Sprintf("o-consistent-%d", i))
		if _, err := f.regSvc.Submit(ctx, userID, eventID, submitReq(idCard, model.DanceTypeClassical, fmt.Sprintf("key-consis-%04d", i))); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	var inflight int
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(1) FROM registrations WHERE group_id = ? AND status IN ('pending_payment','active')`,
		groupID).Scan(&inflight); err != nil {
		t.Fatalf("count in-flight: %v", err)
	}
	if took := groupTaken(t, f.db, groupID); took != inflight || took != len(players) {
		t.Fatalf("taken = %d, in-flight = %d, want both = %d", took, inflight, len(players))
	}
}
