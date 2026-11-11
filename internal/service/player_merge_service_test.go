package service

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/store"
)

// mustAdminID 按用户名取后台账号 ID。
func mustAdminID(t *testing.T, f *regFixture, username string) int64 {
	t.Helper()
	var id int64
	if err := f.db.SQL().QueryRowContext(context.Background(),
		`SELECT id FROM admins WHERE username = ?`, username).Scan(&id); err != nil {
		t.Fatalf("query admin %s: %v", username, err)
	}
	return id
}

// mustInsertPlayerRow 插入一个选手档案并返回 ID（档案合并测试用）。
func mustInsertPlayerRow(t *testing.T, db *store.DB, playerNo, hash string) int64 {
	t.Helper()
	res, err := db.SQL().ExecContext(context.Background(),
		`INSERT INTO players (player_no, name, id_card_hash) VALUES (?, '合并测试', ?)`, playerNo, hash)
	if err != nil {
		t.Fatalf("insert player %s: %v", playerNo, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("player id: %v", err)
	}
	return id
}

// mustInsertBoard 插入一条榜单并返回 ID（成绩/积分冲突测试用）。
func mustInsertBoard(t *testing.T, f *regFixture) int64 {
	t.Helper()
	res, err := f.db.SQL().ExecContext(context.Background(),
		`INSERT INTO boards (season_id, level, dance_type) VALUES (?, 'U11', 'classical')`, f.seasonID)
	if err != nil {
		t.Fatalf("insert board: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("board id: %v", err)
	}
	return id
}

// seedPlayerHistory 为选手写入成绩/积分/报名/资格/奖金各类历史数据。
func seedPlayerHistory(t *testing.T, f *regFixture, playerID, boardID int64) {
	t.Helper()
	eventID, groupID := f.manualEventForMerge(t, "档案合并赛事")
	userID := mustInsertRegUser(t, f.db, "o-merge-"+strconv.FormatInt(playerID, 10))
	execOrFatal(t, f.db, `INSERT INTO registrations
		(event_id, group_id, player_id, user_id, dance_type, status)
		VALUES (?, ?, ?, ?, 'classical', 'active')`, eventID, groupID, playerID, userID)
	execOrFatal(t, f.db, `INSERT INTO results (event_id, board_id, player_id, award)
		VALUES (?, ?, ?, 'champion')`, eventID, boardID, playerID)
	execOrFatal(t, f.db, `INSERT INTO player_points
		(season_id, board_id, player_id, total_points, used_event_ids, computed_at)
		VALUES (?, ?, ?, 100, '[]', '2027-01-02T00:00:00+08:00')`, f.seasonID, boardID, playerID)
	execOrFatal(t, f.db, `INSERT INTO final_qualifications (season_id, board_id, player_id, source)
		VALUES (?, ?, ?, 'auto_direct')`, f.seasonID, boardID, playerID)
	execOrFatal(t, f.db, `INSERT INTO prizes (season_id, event_id, board_id, player_id, award, amount)
		VALUES (?, ?, ?, ?, 'champion', 1000)`, f.seasonID, eventID, boardID, playerID)
}

// manualEventForMerge 创建一个报名中的赛事并返回赛事/组别 ID。
func (f *regFixture) manualEventForMerge(t *testing.T, name string) (eventID, groupID int64) {
	t.Helper()
	eventID = mustCreateRegisteringEvent(t, f, name, []model.AdminEventGroupInput{
		{Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Quota: 10},
	})
	groupID = mustGroupID(t, f.db, eventID, model.GroupLevelU11, model.DanceTypeClassical)
	return eventID, groupID
}

func execOrFatal(t *testing.T, db *store.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.SQL().ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// TestMergePlayer 覆盖档案合并的数据迁移、合并指向与审计日志。
func TestMergePlayer(t *testing.T) {
	f := newRegFixture(t)
	svc := NewPlayerService(f.stores, "test-hmac-key")
	ctx := context.Background()
	adminID := mustAdminID(t, f, "admin")

	sourceID := mustInsertPlayerRow(t, f.db, "SrcPly01", "hash-src-1")
	targetID := mustInsertPlayerRow(t, f.db, "TgtPly01", "hash-tgt-1")
	boardID := mustInsertBoard(t, f)
	seedPlayerHistory(t, f, sourceID, boardID)

	resp, err := svc.Merge(ctx, adminID, sourceID, targetID)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if resp.PlayerID != sourceID || resp.MergedInto != targetID {
		t.Fatalf("resp = %+v", resp)
	}
	for _, table := range []string{"registrations", "results", "player_points",
		"final_qualifications", "prizes"} {
		if resp.Moved[table] != 1 {
			t.Fatalf("moved[%s] = %d, want 1", table, resp.Moved[table])
		}
		var remaining, moved int
		if err := f.db.SQL().QueryRowContext(ctx,
			`SELECT COUNT(1) FROM `+table+` WHERE player_id = ?`, sourceID).Scan(&remaining); err != nil {
			t.Fatalf("count source %s: %v", table, err)
		}
		if err := f.db.SQL().QueryRowContext(ctx,
			`SELECT COUNT(1) FROM `+table+` WHERE player_id = ?`, targetID).Scan(&moved); err != nil {
			t.Fatalf("count target %s: %v", table, err)
		}
		if remaining != 0 || moved == 0 {
			t.Fatalf("%s: remaining=%d moved=%d, want 0/moved", table, remaining, moved)
		}
	}

	var mergedInto sql.NullInt64
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT merged_into FROM players WHERE id = ?`, sourceID).Scan(&mergedInto); err != nil {
		t.Fatalf("query merged_into: %v", err)
	}
	if !mergedInto.Valid || mergedInto.Int64 != targetID {
		t.Fatalf("merged_into = %v, want %d", mergedInto, targetID)
	}
	if n := countAdminLogOf(t, f.db, "admin.player.merge", "player", sourceID); n != 1 {
		t.Fatalf("merge admin logs = %d, want 1", n)
	}

	// 重复合并 / 以已合并档案为目标 / 自身 / 不存在 → 40901 / 40901 / 40001 / 40401。
	if _, err := svc.Merge(ctx, adminID, sourceID, targetID); mustBizCode(t, err) != 40901 {
		t.Fatalf("re-merge should be 40901, got %v", err)
	}
	if _, err := svc.Merge(ctx, adminID, targetID, sourceID); mustBizCode(t, err) != 40901 {
		t.Fatalf("merge into merged target should be 40901, got %v", err)
	}
	if _, err := svc.Merge(ctx, adminID, targetID, targetID); mustBizCode(t, err) != 40001 {
		t.Fatalf("self merge should be 40001, got %v", err)
	}
	if _, err := svc.Merge(ctx, adminID, 999999, targetID); mustBizCode(t, err) != 40401 {
		t.Fatalf("missing player should be 40401, got %v", err)
	}
}

// TestMergePlayerConflict 目标档案已有同赛事成绩时整笔回滚并返回 40901。
func TestMergePlayerConflict(t *testing.T) {
	f := newRegFixture(t)
	svc := NewPlayerService(f.stores, "test-hmac-key")
	ctx := context.Background()
	adminID := mustAdminID(t, f, "admin")

	sourceID := mustInsertPlayerRow(t, f.db, "SrcPly02", "hash-src-2")
	targetID := mustInsertPlayerRow(t, f.db, "TgtPly02", "hash-tgt-2")
	boardID := mustInsertBoard(t, f)
	eventID, _ := f.manualEventForMerge(t, "档案合并冲突")
	execOrFatal(t, f.db, `INSERT INTO results (event_id, board_id, player_id, award)
		VALUES (?, ?, ?, 'champion')`, eventID, boardID, sourceID)
	execOrFatal(t, f.db, `INSERT INTO results (event_id, board_id, player_id, award)
		VALUES (?, ?, ?, 'gold')`, eventID, boardID, targetID)

	if _, err := svc.Merge(ctx, adminID, sourceID, targetID); mustBizCode(t, err) != 40901 {
		t.Fatalf("conflict merge should be 40901, got %v", err)
	}
	var mergedInto sql.NullInt64
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT merged_into FROM players WHERE id = ?`, sourceID).Scan(&mergedInto); err != nil {
		t.Fatalf("query merged_into: %v", err)
	}
	if mergedInto.Valid {
		t.Fatal("conflict merge must roll back merged_into")
	}
	var sourceRows int
	if err := f.db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(1) FROM results WHERE player_id = ?`, sourceID).Scan(&sourceRows); err != nil {
		t.Fatalf("count source results: %v", err)
	}
	if sourceRows != 1 {
		t.Fatalf("source results = %d, want 1 (rollback)", sourceRows)
	}
}
