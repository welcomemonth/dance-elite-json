package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// newEventStoreFixture 打开内存库、跑迁移并返回 store 聚合根。
func newEventStoreFixture(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := Migrate(context.Background(), db.SQL()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	stores := New(db)
	return stores, db.SQL()
}

func ptr[T any](v T) *T { return &v }

func mustInsertSeason(t *testing.T, stores *Store, groupRules string) int64 {
	t.Helper()
	var id int64
	err := stores.WithTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		id, err = stores.Seasons.Insert(context.Background(), tx, SeasonParams{
			Name: "2027 赛季", GroupRules: groupRules,
		})
		return err
	})
	if err != nil {
		t.Fatalf("insert season: %v", err)
	}
	return id
}

const testGroupRules = `{"age_cutoff":"2027-01-01","levels":[
	{"level":"U11","birth_year_from":2016,"birth_year_to":2018},
	{"level":"U13","birth_year_from":2014,"birth_year_to":2015},
	{"level":"U15","birth_year_from":2012,"birth_year_to":2013}]}`

func mustInsertEvent(t *testing.T, stores *Store, seasonID int64, name, status string) int64 {
	t.Helper()
	regStart := "2027-01-01T09:00:00+08:00"
	regEnd := "2027-02-01T18:00:00+08:00"
	lottery := "2027-02-05T10:00:00+08:00"
	material := "2027-03-01T18:00:00+08:00"
	start := "2027-03-15T09:00:00+08:00"
	score := `{"base_points":{"champion":100}}`
	prize := `{"grade_a":[{"rank":1,"amount":100000}]}`
	var id int64
	err := stores.WithTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		id, err = stores.Events.Insert(context.Background(), tx, EventParams{
			SeasonID: seasonID, Name: name, Level: model.EventLevelGradeA,
			Venue: ptr("场馆"), EventDate: ptr("2027-03-15"), Fee: 98000,
			RegStartAt: &regStart, RegEndAt: &regEnd, LotteryAt: &lottery,
			MaterialCheckAt: &material, StartAt: &start,
			Status: status, ScoreRules: &score, PrizeRules: &prize,
		})
		return err
	})
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
	return id
}

func TestSeasonInsertAndUpdate(t *testing.T) {
	stores, _ := newEventStoreFixture(t)
	ctx := context.Background()

	id := mustInsertSeason(t, stores, testGroupRules)
	season, err := stores.Seasons.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if season.Status != model.SeasonStatusUpcoming || season.GroupRules != testGroupRules {
		t.Fatalf("season = %+v", season)
	}

	// 更新名称与状态。
	name := "2027 赛季（改）"
	active := model.SeasonStatusActive
	now := "2027-12-31T23:59:59+08:00"
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Seasons.Update(ctx, tx, id, SeasonPatch{Name: &name, Status: &active, ArchivedAt: &now})
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err := stores.Seasons.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if updated.Name != name || updated.Status != active || updated.ArchivedAt == nil {
		t.Fatalf("updated = %+v", updated)
	}

	// 不存在 → ErrNotFound。
	if _, err := stores.Seasons.Get(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSeasonHasOtherActive(t *testing.T) {
	stores, _ := newEventStoreFixture(t)
	ctx := context.Background()

	id := mustInsertSeason(t, stores, testGroupRules)
	active := model.SeasonStatusActive
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Seasons.Update(ctx, tx, id, SeasonPatch{Status: &active})
	}); err != nil {
		t.Fatalf("activate: %v", err)
	}

	ok, err := stores.Seasons.HasOtherActive(ctx, id)
	if err != nil || ok {
		t.Fatalf("HasOtherActive(exclude self) = %v/%v, want false", ok, err)
	}
	ok, err = stores.Seasons.HasOtherActive(ctx, 9999)
	if err != nil || !ok {
		t.Fatalf("HasOtherActive(other) = %v/%v, want true", ok, err)
	}
}

func TestEventInsertListAndFilters(t *testing.T) {
	stores, _ := newEventStoreFixture(t)
	ctx := context.Background()

	seasonID := mustInsertSeason(t, stores, testGroupRules)
	mustInsertEvent(t, stores, seasonID, "2027 上海甲级赛", model.EventStatusDraft)
	mustInsertEvent(t, stores, seasonID, "2027 全国超级赛", model.EventStatusRegistering)

	total, n, err := stores.Events.List(ctx, EventFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(total) != 2 || n != 2 {
		t.Fatalf("events = %d/%d, want 2/2", len(total), n)
	}

	// 状态筛选。
	list, _, err := stores.Events.List(ctx, EventFilter{Status: model.EventStatusRegistering, Page: 1, PageSize: 10})
	if err != nil || len(list) != 1 || list[0].Name != "2027 全国超级赛" {
		t.Fatalf("status filter = %v/%v", list, err)
	}

	// 关键字筛选。
	list, n, err = stores.Events.List(ctx, EventFilter{Keyword: "上海", Page: 1, PageSize: 10})
	if err != nil || n != 1 || len(list) != 1 {
		t.Fatalf("keyword filter = %v/%d/%v", list, n, err)
	}

	// 分页。
	page1, totalCount, err := stores.Events.List(ctx, EventFilter{Page: 1, PageSize: 1})
	if err != nil || totalCount != 2 || len(page1) != 1 {
		t.Fatalf("page1 = %v/%d", page1, totalCount)
	}
}

func TestEventTransitionStatusCAS(t *testing.T) {
	stores, _ := newEventStoreFixture(t)
	ctx := context.Background()

	seasonID := mustInsertSeason(t, stores, testGroupRules)
	id := mustInsertEvent(t, stores, seasonID, "E", model.EventStatusDraft)

	// draft → registering 成功。
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Events.TransitionStatus(ctx, tx, id,
			[]string{model.EventStatusDraft, model.EventStatusUnpublished}, model.EventStatusRegistering)
	}); err != nil {
		t.Fatalf("transition: %v", err)
	}

	// 已 registering，再按 draft 流转 → ErrConflict。
	err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Events.TransitionStatus(ctx, tx, id,
			[]string{model.EventStatusDraft}, model.EventStatusRegistering)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}

	// 不存在的事件 → ErrNotFound。
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Events.TransitionStatus(ctx, tx, 9999,
			[]string{model.EventStatusDraft}, model.EventStatusRegistering)
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestEventGroupQuotaGuards(t *testing.T) {
	stores, _ := newEventStoreFixture(t)
	ctx := context.Background()

	seasonID := mustInsertSeason(t, stores, testGroupRules)
	eventID := mustInsertEvent(t, stores, seasonID, "E", model.EventStatusRegistering)

	var groupID int64
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		groupID, err = stores.Groups.Insert(ctx, tx, EventGroupParams{
			EventID: eventID, Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical,
			BirthYearFrom: 2016, BirthYearTo: 2018, Quota: 40,
		})
		return err
	}); err != nil {
		t.Fatalf("insert group: %v", err)
	}

	// 调大成功。
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Groups.UpdateQuota(ctx, tx, groupID, 50)
	}); err != nil {
		t.Fatalf("increase quota: %v", err)
	}
	// 调小 → ErrConflict。
	err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Groups.UpdateQuota(ctx, tx, groupID, 30)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("decrease err = %v, want ErrConflict", err)
	}
	// 相等 → ErrConflict（只能调大）。
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Groups.UpdateQuota(ctx, tx, groupID, 50)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("equal err = %v, want ErrConflict", err)
	}

	// 草稿全量编辑可减（SetQuota），但不能小于 taken：先把 taken 模拟占满。
	if _, err := stores.db.sql.ExecContext(ctx,
		`UPDATE event_groups SET taken = 45 WHERE id = ?`, groupID); err != nil {
		t.Fatalf("bump taken: %v", err)
	}
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Groups.SetQuota(ctx, tx, groupID, 40) // < taken 45
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("set below taken err = %v, want ErrConflict", err)
	}
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Groups.SetQuota(ctx, tx, groupID, 46)
	}); err != nil {
		t.Fatalf("set quota: %v", err)
	}

	// 启用开关。
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Groups.SetEnabled(ctx, tx, groupID, false)
	}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Groups.SetEnabled(ctx, tx, 9999, false)
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disable missing err = %v, want ErrNotFound", err)
	}
}

func TestSeasonSingleActiveUniqueIndex(t *testing.T) {
	stores, sqlDB := newEventStoreFixture(t)
	ctx := context.Background()

	idA := mustInsertSeason(t, stores, testGroupRules)
	idB := mustInsertSeason(t, stores, testGroupRules)

	// 绕过 service 直接置 active：部分唯一索引必须挡住第二个 active（TOCTOU 兜底）。
	if _, err := sqlDB.ExecContext(ctx,
		`UPDATE seasons SET status = 'active' WHERE id = ?`, idA); err != nil {
		t.Fatalf("activate A: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx,
		`UPDATE seasons SET status = 'active' WHERE id = ?`, idB); err == nil {
		t.Fatal("第二个 active 赛季应被唯一索引拒绝")
	}

	// A 归档后 B 可以激活。
	if _, err := sqlDB.ExecContext(ctx,
		`UPDATE seasons SET status = 'archived' WHERE id = ?`, idA); err != nil {
		t.Fatalf("archive A: %v", err)
	}
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Seasons.Activate(ctx, tx, idB, model.SeasonStatusUpcoming)
	}); err != nil {
		t.Fatalf("activate B via CAS: %v", err)
	}

	// CAS：从非 upcoming 激活 → ErrConflict；不存在 → ErrNotFound。
	err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Seasons.Activate(ctx, tx, idB, model.SeasonStatusUpcoming)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("re-activate err = %v, want ErrConflict", err)
	}
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Seasons.Activate(ctx, tx, 9999, model.SeasonStatusUpcoming)
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("activate missing err = %v, want ErrNotFound", err)
	}
}
