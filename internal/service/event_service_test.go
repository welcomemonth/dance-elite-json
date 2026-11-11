package service

import (
	"context"
	"errors"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
)

// newEventFixture 打开内存库、跑迁移、种子超管并激活一个赛季。
func newEventFixture(t *testing.T) (*EventService, *SeasonService, *model.AdminSeasonItem) {
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
	return NewEventService(stores), seasonSvc, mustActiveSeason(t, seasonSvc)
}

var validTimeline = model.EventTimeline{
	RegStartAt:      "2027-01-01T09:00:00+08:00",
	RegEndAt:        "2027-02-01T18:00:00+08:00",
	LotteryAt:       "2027-02-05T10:00:00+08:00",
	MaterialCheckAt: "2027-03-01T18:00:00+08:00",
	StartAt:         "2027-03-15T09:00:00+08:00",
}

func createEventReq(seasonID int64, publish bool) model.AdminCreateEventRequest {
	fee := int64(98000)
	return model.AdminCreateEventRequest{
		Name:      "2027 上海甲级赛",
		SeasonID:  seasonID,
		Level:     model.EventLevelGradeA,
		City:      "上海",
		Venue:     "上海国际舞蹈中心",
		EventDate: "2027-03-15",
		Fee:       &fee,
		Timeline:  validTimeline,
		Groups: []model.AdminEventGroupInput{
			{Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Quota: 40},
			{Level: model.GroupLevelU11, DanceType: model.DanceTypeFolk, Quota: 40},
		},
		Publish: publish,
	}
}

func updateEventReq() model.AdminUpdateEventRequest {
	fee := int64(98000)
	return model.AdminUpdateEventRequest{
		Name:      "2027 上海甲级赛（春）",
		Level:     model.EventLevelGradeA,
		City:      "上海",
		Venue:     "上海国际舞蹈中心",
		EventDate: "2027-03-15",
		Fee:       &fee,
		Timeline:  validTimeline,
		Groups: []model.AdminEventGroupInput{
			{Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Quota: 40},
			{Level: model.GroupLevelU11, DanceType: model.DanceTypeFolk, Quota: 40},
		},
	}
}

func bizCode(t *testing.T, err error) int {
	t.Helper()
	var biz *httpx.BizError
	if !errors.As(err, &biz) {
		t.Fatalf("err = %v, want BizError", err)
	}
	return biz.Code
}

func TestEventCreateDraftCopiesBirthYears(t *testing.T) {
	svc, _, season := newEventFixture(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, 1, createEventReq(season.ID, false))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp.Status != model.EventStatusDraft {
		t.Fatalf("status = %q, want draft", resp.Status)
	}

	list, err := svc.List(ctx, model.AdminEventListQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Total != 1 || len(list.List[0].Groups) != 2 {
		t.Fatalf("list = %+v", list)
	}
	// 出生年份从赛季 group_rules（迁移 0007：U11 = 2016-2018）复制。
	for _, g := range list.List[0].Groups {
		if g.Level == model.GroupLevelU11 && (g.BirthYearFrom != 2016 || g.BirthYearTo != 2018) {
			t.Fatalf("U11 birth years = %d-%d, want 2016-2018", g.BirthYearFrom, g.BirthYearTo)
		}
	}
	// 模板预填：score_rules / prize_rules / refund_policy 来自系统设置。
	item := list.List[0]
	if item.ScoreRules == nil || item.PrizeRules == nil || item.RefundPolicy == nil {
		t.Fatalf("rules/policy not prefilled: %+v", item)
	}
}

func TestEventCreateValidation(t *testing.T) {
	svc, _, season := newEventFixture(t)
	ctx := context.Background()

	cases := map[string]func(*model.AdminCreateEventRequest){
		"时间线乱序": func(r *model.AdminCreateEventRequest) {
			r.Timeline.RegEndAt = "2026-12-01T00:00:00+08:00" // 早于 reg_start
		},
		"时间线缺节点": func(r *model.AdminCreateEventRequest) { r.Timeline.LotteryAt = "" },
		"时间线格式错": func(r *model.AdminCreateEventRequest) { r.Timeline.StartAt = "2027-03-15" },
		"级别非法":   func(r *model.AdminCreateEventRequest) { r.Level = "world_cup" },
		"日期格式错":  func(r *model.AdminCreateEventRequest) { r.EventDate = "15/03/2027" },
		"组别重复": func(r *model.AdminCreateEventRequest) {
			r.Groups = append(r.Groups, model.AdminEventGroupInput{
				Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Quota: 10,
			})
		},
		"组别非法": func(r *model.AdminCreateEventRequest) {
			r.Groups[0].Level = "U18"
		},
		"舞种非法": func(r *model.AdminCreateEventRequest) {
			r.Groups[0].DanceType = "ballet"
		},
		"名额为零":     func(r *model.AdminCreateEventRequest) { r.Groups[0].Quota = 0 },
		"规则非法JSON": func(r *model.AdminCreateEventRequest) { r.ScoreRules = "{not-json" },
	}
	for name, mutate := range cases {
		req := createEventReq(season.ID, false)
		mutate(&req)
		if _, err := svc.Create(ctx, 1, req); err == nil {
			t.Fatalf("%s：应被拒绝", name)
		} else if code := bizCode(t, err); code != httpx.CodeBadRequest {
			t.Fatalf("%s：code = %d, want 40001", name, code)
		}
	}
}

func TestEventCreateRequiresActiveSeason(t *testing.T) {
	svc, seasonSvc, season := newEventFixture(t)
	ctx := context.Background()

	// 不存在的赛季 → 40401。
	req := createEventReq(9999, false)
	if _, err := svc.Create(ctx, 1, req); err == nil || bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("err = %v, want 40401", err)
	}

	// upcoming 赛季不能挂赛事 → 40901。
	inactive, err := seasonSvc.Create(ctx, 1, model.AdminSeasonRequest{Name: "2028 赛季"})
	if err != nil {
		t.Fatalf("create inactive season: %v", err)
	}
	req = createEventReq(inactive.ID, false)
	if _, err := svc.Create(ctx, 1, req); err == nil || bizCode(t, err) != httpx.CodeConflict {
		t.Fatalf("err = %v, want 40901", err)
	}
	_ = season
}

func TestEventPublishAndStateMachine(t *testing.T) {
	svc, _, season := newEventFixture(t)
	ctx := context.Background()

	// publish=true 创建即为 registering。
	resp, err := svc.Create(ctx, 1, createEventReq(season.ID, true))
	if err != nil {
		t.Fatalf("create with publish: %v", err)
	}
	if resp.Status != model.EventStatusRegistering {
		t.Fatalf("status = %q, want registering", resp.Status)
	}

	// 重复发布 → 41012。
	if _, err := svc.Publish(ctx, 1, resp.EventID); err == nil || bizCode(t, err) != httpx.CodeEventStateNotAllowed {
		t.Fatalf("republish err = %v, want 41012", err)
	}

	// 关闭报名：registering → reg_closed。
	closed, err := svc.CloseRegistration(ctx, 1, resp.EventID)
	if err != nil {
		t.Fatalf("CloseRegistration: %v", err)
	}
	if closed.Status != model.EventStatusRegClosed {
		t.Fatalf("status = %q", closed.Status)
	}

	// 再次关闭 → 41012。
	if _, err := svc.CloseRegistration(ctx, 1, resp.EventID); err == nil || bizCode(t, err) != httpx.CodeEventStateNotAllowed {
		t.Fatalf("re-close err = %v, want 41012", err)
	}

	// 下架：reg_closed → offline（终止态）。
	offline, err := svc.Offline(ctx, 1, resp.EventID)
	if err != nil {
		t.Fatalf("Offline: %v", err)
	}
	if offline.Status != model.EventStatusOffline {
		t.Fatalf("status = %q", offline.Status)
	}
	// 终止态再流转/编辑 → 41012。
	if _, err := svc.Publish(ctx, 1, resp.EventID); err == nil || bizCode(t, err) != httpx.CodeEventStateNotAllowed {
		t.Fatalf("publish offline err = %v, want 41012", err)
	}
	if _, err := svc.Update(ctx, 1, resp.EventID, updateEventReq()); err == nil || bizCode(t, err) != httpx.CodeEventStateNotAllowed {
		t.Fatalf("update offline err = %v, want 41012", err)
	}
	if _, err := svc.Offline(ctx, 1, resp.EventID); err == nil || bizCode(t, err) != httpx.CodeEventStateNotAllowed {
		t.Fatalf("re-offline err = %v, want 41012", err)
	}
}

func TestEventPublishFromDraft(t *testing.T) {
	svc, _, season := newEventFixture(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, 1, createEventReq(season.ID, false))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	published, err := svc.Publish(ctx, 1, resp.EventID)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if published.Status != model.EventStatusRegistering {
		t.Fatalf("status = %q", published.Status)
	}

	// 关闭全部组别后再发布另一个草稿赛事 → 40001（无启用组别）。
	resp2, err := svc.Create(ctx, 1, createEventReq(season.ID, false))
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	groups, err := svc.stores.Groups.ListByEvent(ctx, resp2.EventID)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	for _, g := range groups {
		if _, err := svc.SetGroupEnabled(ctx, 1, resp2.EventID, g.ID, false); err != nil {
			t.Fatalf("disable group: %v", err)
		}
	}
	if _, err := svc.Publish(ctx, 1, resp2.EventID); err == nil || bizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("publish without enabled group err = %v, want 40001", err)
	}
}

func TestEventUpdatePublishedRestrictions(t *testing.T) {
	svc, _, season := newEventFixture(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, 1, createEventReq(season.ID, true))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	groups, err := svc.stores.Groups.ListByEvent(ctx, resp.EventID)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}

	// 已发布：修改时间线 → 40001。
	req := updateEventReq()
	req.Timeline.StartAt = "2027-03-20T09:00:00+08:00"
	if _, err := svc.Update(ctx, 1, resp.EventID, req); err == nil || bizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("timeline change err = %v, want 40001", err)
	}

	// 已发布：修改级别 → 40001。
	req = updateEventReq()
	req.Level = model.EventLevelSuper
	if _, err := svc.Update(ctx, 1, resp.EventID, req); err == nil || bizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("level change err = %v, want 40001", err)
	}

	// 已发布：名额调小 → 40001。
	req = updateEventReq()
	req.Groups[0] = model.AdminEventGroupInput{GroupID: &groups[0].ID, Quota: 39}
	if _, err := svc.Update(ctx, 1, resp.EventID, req); err == nil || bizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("quota decrease err = %v, want 40001", err)
	}

	// 已发布：新增组别 → 40001。
	req = updateEventReq()
	req.Groups = append(req.Groups, model.AdminEventGroupInput{
		Level: model.GroupLevelU13, DanceType: model.DanceTypeClassical, Quota: 30,
	})
	if _, err := svc.Update(ctx, 1, resp.EventID, req); err == nil || bizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("add group err = %v, want 40001", err)
	}

	// 已发布：调大名额 + 改名 → 成功。
	req = updateEventReq()
	req.Groups[0] = model.AdminEventGroupInput{GroupID: &groups[0].ID, Quota: 50}
	updated, err := svc.Update(ctx, 1, resp.EventID, req)
	if err != nil {
		t.Fatalf("valid update: %v", err)
	}
	if updated.Status != model.EventStatusRegistering {
		t.Fatalf("status = %q", updated.Status)
	}
	group, err := svc.stores.Groups.Get(ctx, groups[0].ID)
	if err != nil {
		t.Fatalf("get group: %v", err)
	}
	if group.Quota != 50 {
		t.Fatalf("quota = %d, want 50", group.Quota)
	}
}

func TestEventDraftUpdateAddsGroup(t *testing.T) {
	svc, _, season := newEventFixture(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, 1, createEventReq(season.ID, false))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	groups, err := svc.stores.Groups.ListByEvent(ctx, resp.EventID)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}

	// 草稿：带 group_id 调名额 + 不带 group_id 新增 U13 组合。
	req := updateEventReq()
	req.Groups = []model.AdminEventGroupInput{
		{GroupID: &groups[0].ID, Quota: 30}, // 草稿可调小
		{Level: model.GroupLevelU13, DanceType: model.DanceTypeClassical, Quota: 20},
	}
	if _, err := svc.Update(ctx, 1, resp.EventID, req); err != nil {
		t.Fatalf("draft update: %v", err)
	}
	after, err := svc.stores.Groups.ListByEvent(ctx, resp.EventID)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if len(after) != 3 {
		t.Fatalf("groups = %d, want 3", len(after))
	}
}

func TestAdjustQuotaRules(t *testing.T) {
	svc, _, season := newEventFixture(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, 1, createEventReq(season.ID, true))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	groups, err := svc.stores.Groups.ListByEvent(ctx, resp.EventID)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	groupID := groups[0].ID

	// 调大成功。
	quota, err := svc.AdjustQuota(ctx, 1, resp.EventID, groupID, 50)
	if err != nil {
		t.Fatalf("AdjustQuota: %v", err)
	}
	if quota.Quota != 50 || quota.Registered != 0 || quota.Remaining != 50 {
		t.Fatalf("quota resp = %+v", quota)
	}

	// 调小/不变 → 40001。
	if _, err := svc.AdjustQuota(ctx, 1, resp.EventID, groupID, 49); err == nil || bizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("decrease err = %v, want 40001", err)
	}
	if _, err := svc.AdjustQuota(ctx, 1, resp.EventID, groupID, 50); err == nil || bizCode(t, err) != httpx.CodeBadRequest {
		t.Fatalf("unchanged err = %v, want 40001", err)
	}

	// 其他赛事的组别 → 40401。
	other, err := svc.Create(ctx, 1, createEventReq(season.ID, false))
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	if _, err := svc.AdjustQuota(ctx, 1, other.EventID, groupID, 60); err == nil || bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("foreign group err = %v, want 40401", err)
	}

	// 下架后不可调整 → 41012。
	if _, err := svc.Offline(ctx, 1, resp.EventID); err != nil {
		t.Fatalf("offline: %v", err)
	}
	if _, err := svc.AdjustQuota(ctx, 1, resp.EventID, groupID, 60); err == nil || bizCode(t, err) != httpx.CodeEventStateNotAllowed {
		t.Fatalf("offline adjust err = %v, want 41012", err)
	}
}

func TestSetGroupEnabled(t *testing.T) {
	svc, _, season := newEventFixture(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, 1, createEventReq(season.ID, true))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	groups, err := svc.stores.Groups.ListByEvent(ctx, resp.EventID)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}

	closed, err := svc.SetGroupEnabled(ctx, 1, resp.EventID, groups[0].ID, false)
	if err != nil {
		t.Fatalf("close group: %v", err)
	}
	if closed.Enabled {
		t.Fatalf("enabled = true, want false")
	}
	reopened, err := svc.SetGroupEnabled(ctx, 1, resp.EventID, groups[0].ID, true)
	if err != nil {
		t.Fatalf("reopen group: %v", err)
	}
	if !reopened.Enabled {
		t.Fatalf("enabled = false, want true")
	}
}

func TestEventListFiltersAndProgress(t *testing.T) {
	svc, _, season := newEventFixture(t)
	ctx := context.Background()

	if _, err := svc.Create(ctx, 1, createEventReq(season.ID, true)); err != nil {
		t.Fatalf("create 1: %v", err)
	}
	// 第二个：超级赛、草稿。
	req := createEventReq(season.ID, false)
	req.Name = "2027 全国超级赛"
	req.Level = model.EventLevelSuper
	if _, err := svc.Create(ctx, 1, req); err != nil {
		t.Fatalf("create 2: %v", err)
	}

	// 级别筛选。
	list, err := svc.List(ctx, model.AdminEventListQuery{Level: model.EventLevelSuper, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list by level: %v", err)
	}
	if list.Total != 1 || list.List[0].Name != "2027 全国超级赛" {
		t.Fatalf("level filter = %+v", list)
	}

	// 关键字 + 状态筛选。
	list, err = svc.List(ctx, model.AdminEventListQuery{Keyword: "上海", Status: model.EventStatusRegistering, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list by keyword/status: %v", err)
	}
	if list.Total != 1 || len(list.List[0].Groups) != 2 {
		t.Fatalf("keyword/status filter = %+v", list)
	}

	// 分页。
	list, err = svc.List(ctx, model.AdminEventListQuery{Page: 2, PageSize: 1})
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if list.Total != 2 || len(list.List) != 1 || list.Page != 2 || list.PageSize != 1 {
		t.Fatalf("pagination = %+v", list)
	}
}

func TestEventTimelineNormalizedToPlus8(t *testing.T) {
	svc, _, season := newEventFixture(t)
	ctx := context.Background()

	// 语义合法但偏移非 +08:00 的时间（此处为 UTC）：必须被接受并规范化为 +08:00 存储，
	// 否则 DB 时间线 CHECK（字符串比较）会拒绝 → 500（审核 #5）。
	req := createEventReq(season.ID, false)
	req.Timeline = model.EventTimeline{
		RegStartAt:      "2027-01-01T01:00:00+00:00", // = 09:00+08:00
		RegEndAt:        "2027-02-01T10:00:00+00:00", // = 18:00+08:00
		LotteryAt:       "2027-02-05T02:00:00+00:00",
		MaterialCheckAt: "2027-03-01T10:00:00+00:00",
		StartAt:         "2027-03-15T01:00:00+00:00",
	}
	resp, err := svc.Create(ctx, 1, req)
	if err != nil {
		t.Fatalf("create with UTC timeline: %v", err)
	}

	item, err := svc.Get(ctx, resp.EventID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item.RegStartAt == nil || *item.RegStartAt != "2027-01-01T09:00:00+08:00" {
		t.Fatalf("reg_start_at = %v, want normalized +08:00", item.RegStartAt)
	}
	if item.RegEndAt == nil || *item.RegEndAt != "2027-02-01T18:00:00+08:00" {
		t.Fatalf("reg_end_at = %v, want normalized +08:00", item.RegEndAt)
	}
}

func TestEventGetDetail(t *testing.T) {
	svc, _, season := newEventFixture(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, 1, createEventReq(season.ID, false))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	item, err := svc.Get(ctx, resp.EventID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item.ID != resp.EventID || item.SeasonID != season.ID || len(item.Groups) != 2 {
		t.Fatalf("item = %+v", item)
	}
	if item.ScoreRules == nil || item.PrizeRules == nil || item.RefundPolicy == nil {
		t.Fatalf("rules/policy not prefilled")
	}

	// 不存在 → 40401。
	if _, err := svc.Get(ctx, 9999); err == nil || bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("err = %v, want 40401", err)
	}
}
