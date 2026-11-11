package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// mustInsertUser 通过原始 SQL 插入小程序用户（T3 store 已覆盖 UpsertLogin，
// 这里只需要一个稳定的 user_id）。
func mustInsertUser(t *testing.T, db *sql.DB, openid string) int64 {
	t.Helper()
	res, err := db.ExecContext(context.Background(),
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

// mustInsertPlayer 通过原始 SQL 插入选手档案（player_store 在 T8 落地）。
func mustInsertPlayer(t *testing.T, db *sql.DB, playerNo, name, idCardHash string) int64 {
	t.Helper()
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO players (player_no, name, id_card_hash) VALUES (?, ?, ?)`,
		playerNo, name, idCardHash)
	if err != nil {
		t.Fatalf("insert player: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("player id: %v", err)
	}
	return id
}

// mustInsertRegistration 通过原始 SQL 插入报名单，返回报名单 ID。
func mustInsertRegistration(t *testing.T, db *sql.DB, eventID, groupID, playerID, userID int64, status string) int64 {
	t.Helper()
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO registrations (event_id, group_id, player_id, user_id, dance_type, status)
		 VALUES (?, ?, ?, ?, 'classical', ?)`,
		eventID, groupID, playerID, userID, status)
	if err != nil {
		t.Fatalf("insert registration: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("registration id: %v", err)
	}
	return id
}

func TestActiveEventIDs(t *testing.T) {
	stores, db := newEventStoreFixture(t)
	ctx := context.Background()

	seasonID := mustInsertSeason(t, stores, testGroupRules)
	eventA := mustInsertEvent(t, stores, seasonID, "赛事A", "registering")
	eventB := mustInsertEvent(t, stores, seasonID, "赛事B", "registering")

	var groupA, groupB int64
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		groupA, err = stores.Groups.Insert(ctx, tx, EventGroupParams{
			EventID: eventA, Level: "U11", DanceType: "classical",
			BirthYearFrom: 2016, BirthYearTo: 2018, Quota: 40,
		})
		if err != nil {
			return err
		}
		groupB, err = stores.Groups.Insert(ctx, tx, EventGroupParams{
			EventID: eventB, Level: "U11", DanceType: "classical",
			BirthYearFrom: 2016, BirthYearTo: 2018, Quota: 40,
		})
		return err
	}); err != nil {
		t.Fatalf("insert groups: %v", err)
	}

	userID := mustInsertUser(t, db, "o-active-1")
	otherUserID := mustInsertUser(t, db, "o-active-2")
	// 同一选手在同一报名单元（player_id, group_id）的在途报名唯一，
	// 因此每个状态用不同选手/player_no 区分。
	for i, status := range []string{"active", "canceled", "pending_payment", "withdrawn"} {
		playerID := mustInsertPlayer(t, db,
			"PN00000"+string(rune('1'+i)), "选手"+string(rune('1'+i)), "hash-"+status)
		mustInsertRegistration(t, db, eventA, groupA, playerID, userID, status)
	}
	// 用户在同一赛事的第二个组别：仍只算一个赛事。
	player2 := mustInsertPlayer(t, db, "PN000009", "选手9", "hash-p2")
	mustInsertRegistration(t, db, eventA, mustInsertSecondGroup(t, stores, eventA), player2, userID, "active")
	// 其他用户的报名不影响当前用户。
	otherPlayer := mustInsertPlayer(t, db, "PN000010", "选手10", "hash-other")
	mustInsertRegistration(t, db, eventB, groupB, otherPlayer, otherUserID, "active")

	got, err := stores.Registrations.ActiveEventIDs(ctx, userID, []int64{eventA, eventB, 9999})
	if err != nil {
		t.Fatalf("ActiveEventIDs: %v", err)
	}
	// 只有 eventA 命中：eventB 无该用户报名，9999 不存在；
	// canceled/pending_payment/withdrawn 均不算"已报名"。
	if len(got) != 1 || !got[eventA] {
		t.Fatalf("ActiveEventIDs = %v, want only event %d", got, eventA)
	}

	// 其他用户只看到 eventB。
	gotOther, err := stores.Registrations.ActiveEventIDs(ctx, otherUserID, []int64{eventA, eventB})
	if err != nil {
		t.Fatalf("ActiveEventIDs(other): %v", err)
	}
	if len(gotOther) != 1 || !gotOther[eventB] {
		t.Fatalf("ActiveEventIDs(other) = %v, want only event %d", gotOther, eventB)
	}

	// 空赛事集合直接返回空 map（列表无赛事时不发 SQL）。
	gotEmpty, err := stores.Registrations.ActiveEventIDs(ctx, userID, nil)
	if err != nil {
		t.Fatalf("ActiveEventIDs(empty): %v", err)
	}
	if len(gotEmpty) != 0 {
		t.Fatalf("ActiveEventIDs(empty) = %v, want empty", gotEmpty)
	}
}

// mustInsertSecondGroup 为同一赛事插入第二个组别（U11 folk），返回其 ID。
func mustInsertSecondGroup(t *testing.T, stores *Store, eventID int64) int64 {
	t.Helper()
	var id int64
	err := stores.WithTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		id, err = stores.Groups.Insert(context.Background(), tx, EventGroupParams{
			EventID: eventID, Level: "U11", DanceType: "folk",
			BirthYearFrom: 2016, BirthYearTo: 2018, Quota: 40,
		})
		return err
	})
	if err != nil {
		t.Fatalf("insert second group: %v", err)
	}
	return id
}

func TestUpdateMaterialsTxGuardAndClear(t *testing.T) {
	stores, db := newEventStoreFixture(t)
	ctx := context.Background()

	seasonID := mustInsertSeason(t, stores, testGroupRules)
	eventID := mustInsertEvent(t, stores, seasonID, "资料赛事", "registering")
	var groupID int64
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		groupID, err = stores.Groups.Insert(ctx, tx, EventGroupParams{
			EventID: eventID, Level: "U11", DanceType: "classical",
			BirthYearFrom: 2016, BirthYearTo: 2018, Quota: 40,
		})
		return err
	}); err != nil {
		t.Fatalf("insert group: %v", err)
	}

	userID := mustInsertUser(t, db, "o-material-store-1")
	playerID := mustInsertPlayer(t, db, "PN100001", "王小舞", "hash-material-1")
	regID := mustInsertRegistration(t, db, eventID, groupID, playerID, userID, "active")

	photo := "uploads/1/PN100001/photo_abcd1234.jpg"
	program := "春闺梦"
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Registrations.UpdateMaterialsTx(ctx, tx, regID, userID, MaterialUpdateParams{
			PhotoPath: &photo, ProgramName: &program, MaterialUpdatedAt: "2027-01-02T20:11:00+08:00",
		})
	}); err != nil {
		t.Fatalf("UpdateMaterialsTx: %v", err)
	}

	var gotPhoto, gotProgram *string
	if err := db.QueryRowContext(ctx,
		`SELECT photo_path, program_name FROM registrations WHERE id = ?`, regID).Scan(&gotPhoto, &gotProgram); err != nil {
		t.Fatalf("query registration: %v", err)
	}
	if gotPhoto == nil || *gotPhoto != photo || gotProgram == nil || *gotProgram != program {
		t.Fatalf("photo=%v program=%v", gotPhoto, gotProgram)
	}

	// 空串清空文本字段（写 NULL），未提交的 photo_path 保持不变。
	empty := ""
	if err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Registrations.UpdateMaterialsTx(ctx, tx, regID, userID, MaterialUpdateParams{
			ProgramName: &empty, MaterialUpdatedAt: "2027-01-02T20:12:00+08:00",
		})
	}); err != nil {
		t.Fatalf("clear program: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT photo_path, program_name FROM registrations WHERE id = ?`, regID).Scan(&gotPhoto, &gotProgram); err != nil {
		t.Fatalf("query cleared registration: %v", err)
	}
	if gotPhoto == nil || *gotPhoto != photo || gotProgram != nil {
		t.Fatalf("after clear photo=%v program=%v", gotPhoto, gotProgram)
	}

	// 非本人或非 active 的报名条件不命中 → ErrConflict。
	otherUserID := mustInsertUser(t, db, "o-material-store-2")
	err := stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Registrations.UpdateMaterialsTx(ctx, tx, regID, otherUserID, MaterialUpdateParams{
			MaterialUpdatedAt: "2027-01-02T20:13:00+08:00",
		})
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("other user err = %v, want ErrConflict", err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE registrations SET status = 'canceled' WHERE id = ?`, regID); err != nil {
		t.Fatalf("cancel registration: %v", err)
	}
	err = stores.WithTx(ctx, func(tx *sql.Tx) error {
		return stores.Registrations.UpdateMaterialsTx(ctx, tx, regID, userID, MaterialUpdateParams{
			MaterialUpdatedAt: "2027-01-02T20:14:00+08:00",
		})
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("canceled registration err = %v, want ErrConflict", err)
	}
}
