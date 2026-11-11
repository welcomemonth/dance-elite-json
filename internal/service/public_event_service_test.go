package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
)

// newPublicEventFixture 打开内存库、种子并装配赛事/小程序赛事服务。
// 时钟可注入：PublicEventService.now 固定返回 currentNow，测试通过修改它控制展示状态推导。
func newPublicEventFixture(t *testing.T) (*EventService, *PublicEventService, *store.Store, *sql.DB, *model.AdminSeasonItem, *time.Time) {
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
	season := mustActiveSeason(t, seasonSvc)

	currentNow := time.Date(2027, 1, 15, 12, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	pub := NewPublicEventService(stores)
	pub.now = func() time.Time { return currentNow }
	return NewEventService(stores), pub, stores, db.SQL(), season, &currentNow
}

// timelineBetween 构造严格递增的五节点时间线（+08:00），报名窗口为 [regStart, regEnd]。
func timelineBetween(regStart, regEnd string) model.EventTimeline {
	return model.EventTimeline{
		RegStartAt:      regStart,
		RegEndAt:        regEnd,
		LotteryAt:       "2027-06-01T10:00:00+08:00",
		MaterialCheckAt: "2027-06-10T10:00:00+08:00",
		StartAt:         "2027-06-15T09:00:00+08:00",
	}
}

// mustCreatePublished 通过业务服务创建并发布一个赛事。
func mustCreatePublished(t *testing.T, svc *EventService, seasonID int64, name string, tl model.EventTimeline) int64 {
	t.Helper()
	req := createEventReq(seasonID, true)
	req.Name = name
	req.Timeline = tl
	created, err := svc.Create(context.Background(), 1, req)
	if err != nil {
		t.Fatalf("create event %s: %v", name, err)
	}
	return created.EventID
}

// mustSetEventStatus 直接置赛事状态（finished 无业务入口，测试专用）。
func mustSetEventStatus(t *testing.T, sqlDB *sql.DB, id int64, status string) {
	t.Helper()
	if _, err := sqlDB.ExecContext(context.Background(),
		`UPDATE events SET status = ? WHERE id = ?`, status, id); err != nil {
		t.Fatalf("set status: %v", err)
	}
}

// mustInsertPublishedRaw 直接经 store 插入已发布赛事（绕开业务校验，用于缓存测试构造数据）。
func mustInsertPublishedRaw(t *testing.T, stores *store.Store, seasonID int64, name string) int64 {
	t.Helper()
	regStart := "2027-01-01T09:00:00+08:00"
	regEnd := "2027-02-01T18:00:00+08:00"
	lottery := "2027-02-05T10:00:00+08:00"
	material := "2027-03-01T18:00:00+08:00"
	start := "2027-03-15T09:00:00+08:00"
	var id int64
	err := stores.WithTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		id, err = stores.Events.Insert(context.Background(), tx, store.EventParams{
			SeasonID: seasonID, Name: name, Level: model.EventLevelGradeA,
			Fee: 98000, RegStartAt: &regStart, RegEndAt: &regEnd, LotteryAt: &lottery,
			MaterialCheckAt: &material, StartAt: &start, Status: model.EventStatusRegistering,
		})
		if err != nil {
			return err
		}
		_, err = stores.Groups.Insert(context.Background(), tx, store.EventGroupParams{
			EventID: id, Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical,
			BirthYearFrom: 2016, BirthYearTo: 2018, Quota: 40,
		})
		return err
	})
	if err != nil {
		t.Fatalf("insert event %s: %v", name, err)
	}
	return id
}

func TestPublicListVisibilityAndStatus(t *testing.T) {
	svc, pub, _, sqlDB, season, _ := newPublicEventFixture(t)
	ctx := context.Background()

	upcomingID := mustCreatePublished(t, svc, season.ID, "未开始报名赛",
		timelineBetween("2027-02-01T09:00:00+08:00", "2027-03-01T18:00:00+08:00"))
	registeringID := mustCreatePublished(t, svc, season.ID, "报名中赛事",
		timelineBetween("2027-01-01T09:00:00+08:00", "2027-02-01T18:00:00+08:00"))
	regClosedID := mustCreatePublished(t, svc, season.ID, "已截止赛事",
		timelineBetween("2026-11-01T09:00:00+08:00", "2027-01-10T18:00:00+08:00"))
	finishedID := mustCreatePublished(t, svc, season.ID, "已结束赛事",
		timelineBetween("2026-09-01T09:00:00+08:00", "2026-10-01T18:00:00+08:00"))
	mustSetEventStatus(t, sqlDB, finishedID, model.EventStatusFinished)

	// draft / offline 均不可见（unpublished 由 offline 入口产生，同样不可见）。
	draftReq := createEventReq(season.ID, false)
	draftReq.Name = "草稿赛事"
	if _, err := svc.Create(ctx, 1, draftReq); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	offlineID := mustCreatePublished(t, svc, season.ID, "已下线赛事",
		timelineBetween("2027-01-01T09:00:00+08:00", "2027-02-01T18:00:00+08:00"))
	if _, err := svc.Offline(ctx, 1, offlineID); err != nil {
		t.Fatalf("offline: %v", err)
	}

	resp, err := pub.List(ctx, model.MiniappEventListQuery{Page: 1, PageSize: 20}, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if resp.Total != 4 {
		t.Fatalf("total = %d, want 4 visible events: %+v", resp.Total, resp.List)
	}
	statusByID := map[int64]string{}
	for _, item := range resp.List {
		statusByID[item.ID] = item.Status
		if len(item.GroupSummary) != 2 {
			t.Fatalf("event %d group_summary = %v, want 2 entries", item.ID, item.GroupSummary)
		}
		if item.RegisteredByMe {
			t.Fatalf("anonymous list item %d registered_by_me = true", item.ID)
		}
	}
	want := map[int64]string{
		upcomingID:    model.MiniappStatusUpcoming,
		registeringID: model.MiniappStatusRegistering,
		regClosedID:   model.MiniappStatusRegClosed,
		finishedID:    model.MiniappStatusFinished,
	}
	for id, st := range want {
		if statusByID[id] != st {
			t.Fatalf("event %d status = %q, want %q (all: %v)", id, statusByID[id], st, statusByID)
		}
	}
}

func TestPublicListStatusFilterAndPagination(t *testing.T) {
	svc, pub, _, _, season, _ := newPublicEventFixture(t)
	ctx := context.Background()

	// 三个报名中赛事 + 一个未开始赛事（now 固定 2027-01-15）。
	for i := 1; i <= 3; i++ {
		mustCreatePublished(t, svc, season.ID, "报名中赛事-"+string(rune('0'+i)),
			timelineBetween("2027-01-01T09:00:00+08:00", "2027-02-01T18:00:00+08:00"))
	}
	mustCreatePublished(t, svc, season.ID, "未开始报名赛",
		timelineBetween("2027-02-01T09:00:00+08:00", "2027-03-01T18:00:00+08:00"))

	// 状态筛选 + 分页：registering 共 3 条，第 2 页（page_size=2）剩 1 条。
	resp, err := pub.List(ctx, model.MiniappEventListQuery{
		Status: model.MiniappStatusRegistering, Page: 2, PageSize: 2,
	}, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if resp.Total != 3 || len(resp.List) != 1 || resp.Page != 2 || resp.PageSize != 2 {
		t.Fatalf("filtered page = %+v, want total 3 / 1 item on page 2", resp)
	}

	upcoming, err := pub.List(ctx, model.MiniappEventListQuery{
		Status: model.MiniappStatusUpcoming, Page: 1, PageSize: 20,
	}, nil)
	if err != nil {
		t.Fatalf("List upcoming: %v", err)
	}
	if upcoming.Total != 1 || upcoming.List[0].Name != "未开始报名赛" {
		t.Fatalf("upcoming = %+v", upcoming)
	}

	// 非法状态值不匹配任何赛事（内存过滤，返回空列表而非报错）。
	none, err := pub.List(ctx, model.MiniappEventListQuery{Status: "bogus"}, nil)
	if err != nil {
		t.Fatalf("List bogus: %v", err)
	}
	if none.Total != 0 || none.List == nil || len(none.List) != 0 {
		t.Fatalf("bogus filter = %+v, want empty non-nil list", none)
	}
}

func TestPublicListRegisteredByMe(t *testing.T) {
	svc, pub, stores, sqlDB, season, _ := newPublicEventFixture(t)
	ctx := context.Background()

	eventID := mustCreatePublished(t, svc, season.ID, "报名中赛事",
		timelineBetween("2027-01-01T09:00:00+08:00", "2027-02-01T18:00:00+08:00"))

	groups, err := stores.Groups.ListByEvent(ctx, eventID)
	if err != nil || len(groups) == 0 {
		t.Fatalf("groups: %v / %d", err, len(groups))
	}

	userID := insertRawUser(t, sqlDB, "o-pub-1")
	activePlayer := insertRawPlayer(t, sqlDB, "PN000011", "活跃选手", "hash-active")
	canceledPlayer := insertRawPlayer(t, sqlDB, "PN000012", "取消选手", "hash-cancel")
	// taken 的维护在 T9 报名下单事务中完成；测试按在途状态同步模拟：
	// active 占名额（taken=1），canceled 不占（已释放）。
	insertRawRegistration(t, sqlDB, eventID, groups[0].ID, activePlayer, userID, "active")
	insertRawRegistration(t, sqlDB, eventID, groups[0].ID, canceledPlayer, userID, "canceled")

	resp, err := pub.List(ctx, model.MiniappEventListQuery{Page: 1, PageSize: 20}, &userID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(resp.List) != 1 {
		t.Fatalf("list = %+v", resp.List)
	}
	item := resp.List[0]
	if !item.RegisteredByMe {
		t.Fatalf("registered_by_me = false, want true (active registration exists)")
	}
	for _, gs := range item.GroupSummary {
		if gs.Level == model.GroupLevelU11 && gs.DanceType == model.DanceTypeClassical {
			if gs.Registered != 1 {
				t.Fatalf("group_summary registered = %d, want 1 (active only)", gs.Registered)
			}
		}
	}

	// 匿名请求同参数返回 registered_by_me=false。
	anon, err := pub.List(ctx, model.MiniappEventListQuery{Page: 1, PageSize: 20}, nil)
	if err != nil {
		t.Fatalf("List anon: %v", err)
	}
	if anon.List[0].RegisteredByMe {
		t.Fatalf("anonymous registered_by_me = true")
	}
}

func TestPublicListCacheBehavior(t *testing.T) {
	_, pub, stores, sqlDB, season, now := newPublicEventFixture(t)
	ctx := context.Background()

	mustInsertPublishedRaw(t, stores, season.ID, "缓存赛事一")

	// 匿名请求写入缓存。
	first, err := pub.List(ctx, model.MiniappEventListQuery{Page: 1, PageSize: 20}, nil)
	if err != nil {
		t.Fatalf("first List: %v", err)
	}
	if first.Total != 1 {
		t.Fatalf("first total = %d", first.Total)
	}

	// 缓存有效期内新增赛事：匿名请求仍命中旧缓存。
	mustInsertPublishedRaw(t, stores, season.ID, "缓存赛事二")
	second, err := pub.List(ctx, model.MiniappEventListQuery{Page: 1, PageSize: 20}, nil)
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if second.Total != 1 {
		t.Fatalf("cached total = %d, want 1 (30s TTL 内不刷新)", second.Total)
	}

	// 带用户 token 的请求绕过缓存，看到实时数据。
	userID := insertRawUser(t, sqlDB, "o-cache-1")
	live, err := pub.List(ctx, model.MiniappEventListQuery{Page: 1, PageSize: 20}, &userID)
	if err != nil {
		t.Fatalf("live List: %v", err)
	}
	if live.Total != 2 {
		t.Fatalf("token total = %d, want 2 (bypass cache)", live.Total)
	}

	// 推进时钟超过 TTL：缓存失效，匿名请求看到新赛事。
	*now = now.Add(31 * time.Second)
	third, err := pub.List(ctx, model.MiniappEventListQuery{Page: 1, PageSize: 20}, nil)
	if err != nil {
		t.Fatalf("third List: %v", err)
	}
	if third.Total != 2 {
		t.Fatalf("after TTL total = %d, want 2", third.Total)
	}

	// 缓存键区分查询参数：不同 page_size 不共享缓存（但同样过期后返回实时数据）。
	other, err := pub.List(ctx, model.MiniappEventListQuery{Page: 1, PageSize: 10}, nil)
	if err != nil {
		t.Fatalf("other List: %v", err)
	}
	if other.Total != 2 || other.PageSize != 10 {
		t.Fatalf("other = %+v", other)
	}
}

func TestPublicDetail(t *testing.T) {
	svc, pub, stores, sqlDB, season, _ := newPublicEventFixture(t)
	ctx := context.Background()

	eventID := mustCreatePublished(t, svc, season.ID, "报名中赛事",
		timelineBetween("2027-01-01T09:00:00+08:00", "2027-02-01T18:00:00+08:00"))
	groups, err := stores.Groups.ListByEvent(ctx, eventID)
	if err != nil {
		t.Fatalf("groups: %v", err)
	}
	userID := insertRawUser(t, sqlDB, "o-detail-1")
	playerID := insertRawPlayer(t, sqlDB, "PN000021", "详情选手", "hash-detail")
	insertRawRegistration(t, sqlDB, eventID, groups[0].ID, playerID, userID, "active")

	detail, err := pub.Detail(ctx, eventID, nil)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if detail.ID != eventID || detail.Status != model.MiniappStatusRegistering {
		t.Fatalf("detail id/status = %d/%s", detail.ID, detail.Status)
	}
	if len(detail.Groups) != 2 {
		t.Fatalf("detail groups = %d, want 2", len(detail.Groups))
	}
	// remaining = quota - taken：被 active 报名占用的组别剩 39，另一组剩 40。
	byKey := map[string]model.MiniappEventGroup{}
	for _, g := range detail.Groups {
		byKey[g.Level+"/"+g.DanceType] = g
		if g.Remaining != g.Quota-g.Registered {
			t.Fatalf("group %+v remaining != quota - registered", g)
		}
	}
	if g := byKey[model.GroupLevelU11+"/"+model.DanceTypeClassical]; g.Registered != 1 || g.Remaining != 39 {
		t.Fatalf("classical group = %+v, want registered 1 / remaining 39", g)
	}
	if detail.Timeline.RegStartAt == nil || *detail.Timeline.RegStartAt != "2027-01-01T09:00:00+08:00" {
		t.Fatalf("timeline = %+v", detail.Timeline)
	}

	// draft / offline / 不存在 → 40401。
	draftReq := createEventReq(season.ID, false)
	draftReq.Name = "草稿赛事"
	draft, err := svc.Create(ctx, 1, draftReq)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	for _, id := range []int64{draft.EventID, 9999} {
		if _, err := pub.Detail(ctx, id, nil); bizCode(t, err) != httpx.CodeNotFound {
			t.Fatalf("detail %d err = %v, want 40401", id, err)
		}
	}
	offlineID := mustCreatePublished(t, svc, season.ID, "下线赛事",
		timelineBetween("2027-01-01T09:00:00+08:00", "2027-02-01T18:00:00+08:00"))
	if _, err := svc.Offline(ctx, 1, offlineID); err != nil {
		t.Fatalf("offline: %v", err)
	}
	if _, err := pub.Detail(ctx, offlineID, nil); bizCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("offline detail err = %v, want 40401", err)
	}
}

// ---- 原始 SQL 辅助（registration/player 写路径在 T8/T9 落地）----

func insertRawUser(t *testing.T, sqlDB *sql.DB, openid string) int64 {
	t.Helper()
	res, err := sqlDB.ExecContext(context.Background(),
		`INSERT INTO users (openid) VALUES (?)`, openid)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("user id: %v", err)
	}
	return id
}

func insertRawPlayer(t *testing.T, sqlDB *sql.DB, playerNo, name, hash string) int64 {
	t.Helper()
	res, err := sqlDB.ExecContext(context.Background(),
		`INSERT INTO players (player_no, name, id_card_hash) VALUES (?, ?, ?)`,
		playerNo, name, hash)
	if err != nil {
		t.Fatalf("insert player: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("player id: %v", err)
	}
	return id
}

// insertRawRegistration 插入报名单并按在途状态同步 taken（T9 将在下单事务内维护 taken）。
func insertRawRegistration(t *testing.T, sqlDB *sql.DB, eventID, groupID, playerID, userID int64, status string) {
	t.Helper()
	delta := 0
	if status == "active" || status == "pending_payment" {
		delta = 1
	}
	if _, err := sqlDB.ExecContext(context.Background(),
		`UPDATE event_groups SET taken = taken + ? WHERE id = ?`, delta, groupID); err != nil {
		t.Fatalf("bump taken: %v", err)
	}
	if _, err := sqlDB.ExecContext(context.Background(),
		`INSERT INTO registrations (event_id, group_id, player_id, user_id, dance_type, status)
		 VALUES (?, ?, ?, ?, 'classical', ?)`,
		eventID, groupID, playerID, userID, status); err != nil {
		t.Fatalf("insert registration: %v", err)
	}
}
