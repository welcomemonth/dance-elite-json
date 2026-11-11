package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// waitlistFixture 创建赛季/赛事/组别并返回赛事与组别 ID。
func waitlistFixture(t *testing.T) (*Store, *sql.DB, int64, int64) {
	t.Helper()
	stores, db := newEventStoreFixture(t)
	seasonID := mustInsertSeason(t, stores, testGroupRules)
	eventID := mustInsertEvent(t, stores, seasonID, "候补赛事", "registering")
	groupID := mustInsertGroup(t, stores, eventID, model.GroupLevelU11, model.DanceTypeClassical, 1)
	return stores, db, eventID, groupID
}

// mustInsertWaitlist 在事务内插入一条 waiting 候补并返回 id/position。
func mustInsertWaitlist(t *testing.T, stores *Store, eventID, groupID, userID int64, name, hash, danceType string) (int64, int) {
	t.Helper()
	ctx := context.Background()
	var id int64
	var position int
	err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		position, err = stores.Waitlists.NextPositionTx(ctx, tx, eventID, groupID)
		if err != nil {
			return err
		}
		id, err = stores.Waitlists.InsertTx(ctx, tx, WaitlistParams{
			EventID: eventID, GroupID: groupID, UserID: userID,
			DanceType: danceType, Name: name,
			IDCardHash: ptr(hash), IDCardMasked: ptr("3301****" + hash[len(hash)-4:]),
			Phone: "13800000000", PositionNo: position,
		})
		return err
	})
	if err != nil {
		t.Fatalf("insert waitlist %s: %v", name, err)
	}
	return id, position
}

// TestWaitlistInsertAndQueue 覆盖入队序号、重复排队唯一约束、用户列表与取消。
func TestWaitlistInsertAndQueue(t *testing.T) {
	stores, db, eventID, groupID := waitlistFixture(t)
	ctx := context.Background()
	userA := mustInsertUser(t, db, "wait-user-a")
	userB := mustInsertUser(t, db, "wait-user-b")

	idA, posA := mustInsertWaitlist(t, stores, eventID, groupID, userA, "甲", "hash-a", model.DanceTypeClassical)
	idB, posB := mustInsertWaitlist(t, stores, eventID, groupID, userB, "乙", "hash-b", model.DanceTypeClassical)
	if posA != 1 || posB != 2 {
		t.Fatalf("positions = %d, %d, want 1, 2", posA, posB)
	}

	// 同报名单元同证件号 waiting 重复 → 唯一冲突（部分唯一索引）。
	err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := stores.Waitlists.InsertTx(ctx, tx, WaitlistParams{
			EventID: eventID, GroupID: groupID, UserID: userA,
			DanceType: model.DanceTypeClassical, Name: "甲",
			IDCardHash: ptr("hash-a"), Phone: "13800000000", PositionNo: 3,
		})
		return err
	})
	if !IsUniqueViolation(err) {
		t.Fatalf("duplicate waiting insert err = %v, want unique violation", err)
	}

	exists, err := waitlistExistsTx(t, stores, eventID, groupID, "hash-a")
	if err != nil || !exists {
		t.Fatalf("WaitingExistsTx = %v, %v, want true", exists, err)
	}

	// 我的候补按 id 倒序，仅返回本人。
	rows, total, err := stores.Waitlists.ListByUser(ctx, userA, 1, 20)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].WaitlistID != idA || rows[0].Level != model.GroupLevelU11 {
		t.Fatalf("my waitlists = %+v total=%d", rows, total)
	}

	// 取消本人 waiting → given_up + handled_at；重复取消条件未命中（ErrConflict）。
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Waitlists.CancelOwnTx(ctx, tx, idA, userA, "2027-01-01T10:00:00+08:00")
	})
	if err != nil {
		t.Fatalf("cancel own: %v", err)
	}
	w, err := stores.Waitlists.Get(ctx, idA)
	if err != nil || w.Status != model.WaitlistStatusGivenUp || w.HandledAt == nil {
		t.Fatalf("cancelled waitlist = %+v, %v", w, err)
	}
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Waitlists.CancelOwnTx(ctx, tx, idA, userA, "2027-01-01T10:00:00+08:00")
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("repeat cancel err = %v, want ErrConflict", err)
	}
	// 非本人取消 → ErrConflict（status waiting 但 user 不匹配）。
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Waitlists.CancelOwnTx(ctx, tx, idB, userA, "2027-01-01T10:00:00+08:00")
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("cancel other's err = %v, want ErrConflict", err)
	}

	// given_up 后同证件号重新排队：唯一索引只约束 waiting，允许插入且序号继续递增。
	_, posAgain := mustInsertWaitlist(t, stores, eventID, groupID, userA, "甲", "hash-a", model.DanceTypeClassical)
	if posAgain != 3 {
		t.Fatalf("re-queue position = %d, want 3", posAgain)
	}
}

// waitlistExistsTx 在事务内调用 WaitingExistsTx 的小工具。
func waitlistExistsTx(t *testing.T, stores *Store, eventID, groupID int64, hash string) (bool, error) {
	t.Helper()
	var exists bool
	err := stores.WithTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		exists, err = stores.Waitlists.WaitingExistsTx(context.Background(), tx, eventID, groupID, hash)
		return err
	})
	return exists, err
}

// TestWaitlistAdminListAndReleasedQuota 覆盖后台列表筛选、排序与释放名额统计。
func TestWaitlistAdminListAndReleasedQuota(t *testing.T) {
	stores, db, eventID, classicalID := waitlistFixture(t)
	ctx := context.Background()
	userA := mustInsertUser(t, db, "admin-list-a")
	userB := mustInsertUser(t, db, "admin-list-b")

	folkID := mustInsertGroup(t, stores, eventID, model.GroupLevelU11, model.DanceTypeFolk, 1)
	mustInsertWaitlist(t, stores, eventID, classicalID, userA, "古典甲", "hash-c1", model.DanceTypeClassical)
	mustInsertWaitlist(t, stores, eventID, classicalID, userB, "古典乙", "hash-c2", model.DanceTypeClassical)
	mustInsertWaitlist(t, stores, eventID, folkID, userA, "民族甲", "hash-f1", model.DanceTypeFolk)

	// 古典组满员，民族组释放 1 个名额（quota=1, taken=0）。
	exec(t, db, `UPDATE event_groups SET taken = quota WHERE id = ?`, classicalID)
	released, err := stores.Waitlists.ReleasedQuota(ctx, eventID, model.GroupLevelU11, model.DanceTypeFolk)
	if err != nil || released != 1 {
		t.Fatalf("released quota = %d, %v, want 1", released, err)
	}
	if released, _ := stores.Waitlists.ReleasedQuota(ctx, 0, "", ""); released != 0 {
		t.Fatalf("released quota without event = %d, want 0", released)
	}

	rows, total, err := stores.Waitlists.ListAdmin(ctx, AdminWaitlistFilter{
		EventID: eventID, Level: model.GroupLevelU11, DanceType: model.DanceTypeClassical, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListAdmin: %v", err)
	}
	if total != 2 || len(rows) != 2 {
		t.Fatalf("filtered total = %d rows=%d, want 2", total, len(rows))
	}
	if rows[0].PositionNo != 1 || rows[0].Name != "古典甲" || rows[1].PositionNo != 2 {
		t.Fatalf("queue order = %+v", rows)
	}
	if rows[0].IDCardMasked == nil || rows[0].Phone != "13800000000" {
		t.Fatalf("admin row = %+v", rows[0])
	}

	// status 筛选。
	if _, total, _ := stores.Waitlists.ListAdmin(ctx, AdminWaitlistFilter{
		EventID: eventID, Status: model.WaitlistStatusWaiting, Page: 1, PageSize: 20,
	}); total != 3 {
		t.Fatalf("waiting total = %d, want 3", total)
	}
}

// TestWaitlistTransitions 覆盖通知与终态流转的条件语义。
func TestWaitlistTransitions(t *testing.T) {
	stores, db, eventID, groupID := waitlistFixture(t)
	ctx := context.Background()
	userID := mustInsertUser(t, db, "transition-user")
	id, _ := mustInsertWaitlist(t, stores, eventID, groupID, userID, "甲", "hash-t", model.DanceTypeClassical)

	err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Waitlists.NotifyTx(ctx, tx, id, "2027-01-02T10:00:00+08:00")
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	w, _ := stores.Waitlists.Get(ctx, id)
	if w.Status != model.WaitlistStatusNotified || w.NotifiedAt == nil {
		t.Fatalf("notified waitlist = %+v", w)
	}

	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Waitlists.FinishTx(ctx, tx, id, model.WaitlistStatusConverted, "2027-01-03T10:00:00+08:00")
	})
	if err != nil {
		t.Fatalf("finish converted: %v", err)
	}
	// 终态不可再次流转。
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Waitlists.NotifyTx(ctx, tx, id, "2027-01-04T10:00:00+08:00")
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("notify converted err = %v, want ErrConflict", err)
	}
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Waitlists.FinishTx(ctx, tx, id, model.WaitlistStatusRemoved, "2027-01-04T10:00:00+08:00")
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("finish removed after converted err = %v, want ErrConflict", err)
	}
}
