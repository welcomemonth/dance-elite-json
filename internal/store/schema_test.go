package store

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// openMigrated 返回已执行全部迁移的内存库。
func openMigrated(t *testing.T) *DB {
	t.Helper()
	db := openMemory(t)
	if err := Migrate(context.Background(), db.SQL()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func exec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// schemaColumns 是 docs/优舞盟报名功能/数据库表结构设计.md 逐表比对清单。
// 新增迁移改表时同步更新，避免文档与 schema 悄悄漂移。
var schemaColumns = map[string][]string{
	"admins": {
		"id", "username", "name", "password_hash", "role", "status", "last_login_at",
		"created_at", "updated_at",
	},
	"admin_logs": {
		"id", "admin_id", "action", "target_type", "target_id", "detail",
		"created_at", "updated_at",
	},
	"settings": {"key", "value", "created_at", "updated_at"},
	"users": {
		"id", "openid", "unionid", "nickname", "avatar_url", "phone", "status",
		"last_login_at", "created_at", "updated_at",
	},
	"players": {
		"id", "player_no", "name", "id_card_hash", "id_type", "gender", "birth_date",
		"avatar_path", "merged_into", "created_at", "updated_at",
	},
	"seasons": {
		"id", "name", "start_date", "end_date", "status", "group_rules",
		"archived_at", "created_at", "updated_at",
	},
	"events": {
		"id", "season_id", "name", "level", "city", "venue", "event_date", "fee",
		"cover_path", "reg_start_at", "reg_end_at", "lottery_at", "material_check_at",
		"start_at", "status", "score_rules", "prize_rules", "refund_policy",
		"work_duration_limit", "process_desc", "honor_desc", "exclude_from_prize",
		"audio_review_token", "created_at", "updated_at",
	},
	"event_groups": {
		"id", "event_id", "level", "dance_type", "birth_year_from", "birth_year_to",
		"quota", "taken", "enabled", "created_at", "updated_at",
	},
	"registrations": {
		"id", "event_id", "group_id", "player_id", "user_id", "dance_type",
		"institution", "status", "source", "cs_ticket_id", "photo_path", "audio_path",
		"program_name", "teacher_name", "contact_phone", "express_address",
		"work_intro", "special_req", "material_updated_at", "created_at", "updated_at",
	},
	"orders": {
		"id", "order_no", "registration_id", "user_id", "amount", "status",
		"request_id", "prepay_id", "paid_at", "canceled_at", "cancel_reason", "created_at", "updated_at",
	},
	"payments": {
		"id", "order_id", "type", "prepay_id", "transaction_id", "refund_id",
		"amount", "status", "raw_callback", "notified_at", "created_at", "updated_at",
	},
	"refund_tickets": {
		"id", "ticket_no", "order_id", "reason", "applicant", "status",
		"handler_id", "reject_reason", "handled_at", "created_by", "created_at", "updated_at",
	},
	"waitlists": {
		"id", "event_id", "group_id", "user_id", "dance_type", "name", "id_card_hash",
		"id_card_masked", "institution", "phone", "status", "position_no", "notified_at",
		"handled_at", "created_at", "updated_at",
	},
	"cs_tickets": {
		"id", "type", "user_name", "phone", "content", "related_order_id",
		"related_registration_id", "status", "remark", "created_by",
		"created_at", "updated_at",
	},
	"boards": {
		"id", "season_id", "level", "dance_type", "status", "published_at",
		"created_at", "updated_at",
	},
	"results": {
		"id", "event_id", "board_id", "player_id", "institution", "award",
		"score_a", "score_b", "score_c", "score_total",
		"final_score_a", "final_score_b", "final_score_c", "final_score_total",
		"points", "prize_amount", "advanced_to_final", "remark", "status",
		"import_batch", "created_by", "created_at", "updated_at",
	},
	"player_points": {
		"id", "season_id", "board_id", "player_id", "total_points", "used_event_ids",
		"rank", "display_name", "hidden", "computed_at", "created_at", "updated_at",
	},
	"institution_points": {
		"id", "season_id", "institution_name", "total_points", "player_count",
		"rank", "computed_at", "created_at", "updated_at",
	},
	"final_qualifications": {
		"id", "season_id", "board_id", "player_id", "source", "reason", "status",
		"created_by", "created_at", "updated_at",
	},
	"prizes": {
		"id", "season_id", "event_id", "board_id", "player_id", "institution_name",
		"award", "amount", "status", "granted_at", "grantee_note",
		"created_at", "updated_at",
	},
}

// TestSchemaMatchesDesignDoc 迁移后的表与列必须与《数据库表结构设计》一致。
func TestSchemaMatchesDesignDoc(t *testing.T) {
	db := openMigrated(t)

	for table, wantCols := range schemaColumns {
		got := tableColumns(t, db.SQL(), table)
		for _, col := range wantCols {
			if !got[col] {
				t.Errorf("table %s is missing column %s (have %v)", table, col, keys(got))
			}
		}
		if !got["created_at"] || !got["updated_at"] {
			t.Errorf("table %s must have created_at/updated_at", table)
		}
	}
}

// TestForeignKeyCheckClean 各域写入一行合法数据后 foreign_key_check 不得报违规。
// 同时验证外键确实被启用（非法引用必须被拒绝）。
func TestForeignKeyCheckClean(t *testing.T) {
	db := openMigrated(t)
	sqlDB := db.SQL()

	insertFixtures(t, sqlDB)

	rows, err := sqlDB.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var table, parent string
		var rowid int64
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			t.Fatalf("scan foreign_key_check: %v", err)
		}
		t.Errorf("foreign key violation: table=%s rowid=%d parent=%s fkid=%d", table, rowid, parent, fkid)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("foreign_key_check rows: %v", err)
	}

	if _, err := sqlDB.Exec(
		`INSERT INTO orders (order_no, registration_id, user_id, amount, status) VALUES ('BAD-FK', 9999, 1, 100, 'pending_payment')`); err == nil {
		t.Fatal("expected foreign key violation for registration_id=9999")
	}
}

// TestRegistrationInFlightUnique 同一选手同一报名单元只允许一条 pending_payment/active 报名；
// canceled 记录不占用唯一约束。
func TestRegistrationInFlightUnique(t *testing.T) {
	db := openMigrated(t)
	sqlDB := db.SQL()
	insertFixtures(t, sqlDB)

	// fixture 中已有 (player 1, group 1) 的 active 报名，再插 pending_payment 应被拒。
	_, err := sqlDB.Exec(`INSERT INTO registrations
		(event_id, group_id, player_id, user_id, dance_type, status)
		VALUES (1, 1, 1, 1, 'classical', 'pending_payment')`)
	if err == nil {
		t.Fatal("expected unique violation for second in-flight registration")
	}

	// canceled 不占唯一约束。
	exec(t, sqlDB, `INSERT INTO registrations (id, event_id, group_id, player_id, user_id, dance_type, status)
		VALUES (2, 1, 1, 1, 1, 'classical', 'canceled')`)

	// 已取消的报名不能直接复活为 active（会与进行中报名冲突）。
	_, err = sqlDB.Exec(`UPDATE registrations SET status = 'active' WHERE id = 2`)
	if err == nil {
		t.Fatal("expected unique violation when reviving canceled registration")
	}
}

// TestOrderRequestIdempotency 同一用户同一 request_id 只允许一个订单；NULL request_id 不受限。
func TestOrderRequestIdempotency(t *testing.T) {
	db := openMigrated(t)
	sqlDB := db.SQL()
	insertFixtures(t, sqlDB)

	_, err := sqlDB.Exec(`INSERT INTO orders (order_no, registration_id, user_id, amount, status, request_id)
		VALUES ('ORD-DUP', 1, 1, 98000, 'pending_payment', 'req-1')`)
	if err == nil {
		t.Fatal("expected unique violation for duplicate (user_id, request_id)")
	}

	exec(t, sqlDB, `INSERT INTO orders (order_no, registration_id, user_id, amount, status)
		VALUES ('ORD-NULL-1', 1, 1, 98000, 'pending_payment')`)
	exec(t, sqlDB, `INSERT INTO orders (order_no, registration_id, user_id, amount, status)
		VALUES ('ORD-NULL-2', 1, 1, 98000, 'pending_payment')`)
}

// TestEventGroupTakenAtomicUpdate 抢名额必须用原子条件 UPDATE，满员后 RowsAffected=0。
func TestEventGroupTakenAtomicUpdate(t *testing.T) {
	db := openMigrated(t)
	sqlDB := db.SQL()
	insertFixtures(t, sqlDB)

	exec(t, sqlDB, `UPDATE event_groups SET quota = 1, taken = 0 WHERE id = 1`)

	res, err := sqlDB.Exec(`UPDATE event_groups SET taken = taken + 1 WHERE id = 1 AND taken < quota`)
	if err != nil {
		t.Fatalf("take quota: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("first take affected %d rows, want 1", n)
	}
	res, err = sqlDB.Exec(`UPDATE event_groups SET taken = taken + 1 WHERE id = 1 AND taken < quota`)
	if err != nil {
		t.Fatalf("take quota again: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 0 {
		t.Fatalf("second take affected %d rows, want 0 (quota full)", n)
	}

	// taken 不能为负（取消报名回退时的保护）。
	if _, err := sqlDB.Exec(`UPDATE event_groups SET taken = -1 WHERE id = 1`); err == nil {
		t.Fatal("expected CHECK violation for negative taken")
	}
}

// TestPaymentIdempotencyKeys transaction_id 与 refund_id 的幂等唯一约束。
func TestPaymentIdempotencyKeys(t *testing.T) {
	db := openMigrated(t)
	sqlDB := db.SQL()
	insertFixtures(t, sqlDB)

	if _, err := sqlDB.Exec(`INSERT INTO payments (order_id, type, transaction_id, amount, status)
		VALUES (1, 'pay', 'txn-1', 98000, 'success')`); err == nil {
		t.Fatal("expected unique violation for duplicate transaction_id")
	}
	exec(t, sqlDB, `INSERT INTO payments (order_id, type, refund_id, amount, status)
		VALUES (1, 'refund', 'rf-1', 98000, 'success')`)
	if _, err := sqlDB.Exec(`INSERT INTO payments (order_id, type, refund_id, amount, status)
		VALUES (1, 'refund', 'rf-1', 98000, 'success')`); err == nil {
		t.Fatal("expected unique violation for duplicate refund_id")
	}
	// 多条 NULL refund_id（普通支付流水）可共存。
	exec(t, sqlDB, `INSERT INTO payments (order_id, type, amount, status) VALUES (1, 'pay', 98000, 'closed')`)
}

// TestResultsPublishedUnique 有效成绩唯一；纠错保留旧记录（corrected）不冲突。
func TestResultsPublishedUnique(t *testing.T) {
	db := openMigrated(t)
	sqlDB := db.SQL()
	insertFixtures(t, sqlDB)

	// fixture 中已有 (event 1, board 1, player 1) 的 published 成绩。
	if _, err := sqlDB.Exec(`INSERT INTO results (event_id, board_id, player_id, award)
		VALUES (1, 1, 1, 'runner_up')`); err == nil {
		t.Fatal("expected unique violation for second published result")
	}

	// 纠错：旧记录置 corrected，再发布新记录。
	exec(t, sqlDB, `UPDATE results SET status = 'corrected' WHERE id = 1`)
	exec(t, sqlDB, `INSERT INTO results (event_id, board_id, player_id, award)
		VALUES (1, 1, 1, 'runner_up')`)
}

// TestEventTimelineOrderEnforced 时间线 5 节点在非空时必须严格先后。
func TestEventTimelineOrderEnforced(t *testing.T) {
	db := openMigrated(t)
	sqlDB := db.SQL()
	insertFixtures(t, sqlDB)

	_, err := sqlDB.Exec(`INSERT INTO events (season_id, name, level, reg_start_at, reg_end_at)
		VALUES (1, '乱序赛事', 'grade_a', '2027-03-02T00:00:00+08:00', '2027-03-01T00:00:00+08:00')`)
	if err == nil {
		t.Fatal("expected CHECK violation for reg_start_at >= reg_end_at")
	}

	exec(t, sqlDB, `INSERT INTO events (id, season_id, name, level,
		reg_start_at, reg_end_at, lottery_at, material_check_at, start_at)
		VALUES (2, 1, '有序赛事', 'super',
		'2027-01-01T00:00:00+08:00', '2027-02-01T00:00:00+08:00',
		'2027-02-10T00:00:00+08:00', '2027-02-20T00:00:00+08:00',
		'2027-03-01T00:00:00+08:00')`)
}

var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\+08:00$`)

// TestTimestampsAutoManaged 插入自动写时间戳；更新自动刷新 updated_at 且不改 created_at。
func TestTimestampsAutoManaged(t *testing.T) {
	db := openMigrated(t)
	sqlDB := db.SQL()

	exec(t, sqlDB, `INSERT INTO users (openid) VALUES ('o-auto')`)
	var createdAt, updatedAt string
	if err := sqlDB.QueryRow(`SELECT created_at, updated_at FROM users WHERE openid = 'o-auto'`).
		Scan(&createdAt, &updatedAt); err != nil {
		t.Fatalf("query timestamps: %v", err)
	}
	if !timestampPattern.MatchString(createdAt) || !timestampPattern.MatchString(updatedAt) {
		t.Fatalf("timestamps not RFC3339 local: created=%q updated=%q", createdAt, updatedAt)
	}

	// 用固定的历史时间戳插入，更新后 updated_at 必须被触发器刷新，created_at 保持不变。
	exec(t, sqlDB, `INSERT INTO users (openid, created_at, updated_at)
		VALUES ('o-old', '2000-01-01T00:00:00+08:00', '2000-01-01T00:00:00+08:00')`)
	exec(t, sqlDB, `UPDATE users SET nickname = '新昵称' WHERE openid = 'o-old'`)
	if err := sqlDB.QueryRow(`SELECT created_at, updated_at FROM users WHERE openid = 'o-old'`).
		Scan(&createdAt, &updatedAt); err != nil {
		t.Fatalf("query timestamps after update: %v", err)
	}
	if createdAt != "2000-01-01T00:00:00+08:00" {
		t.Fatalf("created_at changed on update: %q", createdAt)
	}
	if updatedAt == "2000-01-01T00:00:00+08:00" || !timestampPattern.MatchString(updatedAt) {
		t.Fatalf("updated_at not refreshed by trigger: %q", updatedAt)
	}
}

// TestSeedAdminIdempotent 超管种子可重复执行，且已存在账号不会被覆盖。
func TestSeedAdminIdempotent(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	if err := db.Seed(ctx, SeedOptions{}); err != nil {
		t.Fatalf("seed with empty password: %v", err)
	}
	var n int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM admins`).Scan(&n); err != nil {
		t.Fatalf("count admins: %v", err)
	}
	if n != 0 {
		t.Fatalf("empty password created %d admins, want 0", n)
	}

	const password = "Init@2027"
	if err := db.Seed(ctx, SeedOptions{AdminPassword: password}); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	var username, name, hash, role string
	if err := db.SQL().QueryRow(`SELECT username, name, password_hash, role FROM admins`).
		Scan(&username, &name, &hash, &role); err != nil {
		t.Fatalf("query admin: %v", err)
	}
	if username != DefaultAdminUsername || name != DefaultAdminName || role != "super_admin" {
		t.Fatalf("admin = %q name=%q role=%q, want %q %q super_admin",
			username, name, role, DefaultAdminUsername, DefaultAdminName)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		t.Fatalf("password hash does not match: %v", err)
	}

	// 再次种子（不同密码）不得改变已有账号。
	if err := db.Seed(ctx, SeedOptions{AdminPassword: "Different@2027"}); err != nil {
		t.Fatalf("re-seed admin: %v", err)
	}
	var hashAfter string
	if err := db.SQL().QueryRow(`SELECT password_hash FROM admins WHERE username = ?`, DefaultAdminUsername).
		Scan(&hashAfter); err != nil {
		t.Fatalf("query admin after re-seed: %v", err)
	}
	if hashAfter != hash {
		t.Fatal("re-seed overwrote existing admin password hash")
	}
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM admins`).Scan(&n); err != nil {
		t.Fatalf("count admins after re-seed: %v", err)
	}
	if n != 1 {
		t.Fatalf("admins after re-seed = %d, want 1", n)
	}

	// 0008 迁移前创建的历史账号 name 为空：再次种子只补显示名，不改密码。
	exec(t, db.SQL(), `UPDATE admins SET name = NULL WHERE username = ?`, DefaultAdminUsername)
	if err := db.Seed(ctx, SeedOptions{AdminPassword: "Different@2027"}); err != nil {
		t.Fatalf("re-seed to backfill name: %v", err)
	}
	if err := db.SQL().QueryRow(`SELECT name, password_hash FROM admins WHERE username = ?`,
		DefaultAdminUsername).Scan(&name, &hashAfter); err != nil {
		t.Fatalf("query admin after name backfill: %v", err)
	}
	if name != DefaultAdminName {
		t.Fatalf("name after backfill = %q, want %q", name, DefaultAdminName)
	}
	if hashAfter != hash {
		t.Fatal("name backfill overwrote existing password hash")
	}

	if err := db.Seed(ctx, SeedOptions{AdminPassword: string(make([]byte, 73))}); err == nil {
		t.Fatal("expected error for password longer than bcrypt limit")
	}
}

// TestSeedSettingsDefaults 迁移应写入可解析的默认配置模板。
func TestSeedSettingsDefaults(t *testing.T) {
	db := openMigrated(t)

	wantKeys := []string{
		"group_rules",
		"refund_policy",
		"score_rules_template",
		"prize_rules_template",
	}
	for _, key := range wantKeys {
		var valid int
		if err := db.SQL().QueryRow(`SELECT json_valid(value) FROM settings WHERE "key" = ?`, key).Scan(&valid); err != nil {
			t.Fatalf("settings %q missing: %v", key, err)
		}
		if valid != 1 {
			t.Fatalf("settings %q is not valid JSON", key)
		}
	}
}

// insertFixtures 写入各域一行互相关联的合法数据，供约束测试复用。
func insertFixtures(t *testing.T, db *sql.DB) {
	t.Helper()
	exec(t, db, `INSERT INTO admins (id, username, password_hash, role)
		VALUES (1, 'admin', 'hash', 'super_admin')`)
	exec(t, db, `INSERT INTO users (id, openid) VALUES (1, 'openid-1')`)
	exec(t, db, `INSERT INTO players (id, player_no, name, id_card_hash)
		VALUES (1, 'Ab12Cd34', '张三', 'hash-1')`)
	exec(t, db, `INSERT INTO seasons (id, name, start_date, end_date, status)
		VALUES (1, '2027 赛季', '2027-01-01', '2027-12-31', 'active')`)
	exec(t, db, `INSERT INTO events (id, season_id, name, level, fee)
		VALUES (1, 1, '2027 上海甲级赛', 'grade_a', 98000)`)
	exec(t, db, `INSERT INTO event_groups (id, event_id, level, dance_type, birth_year_from, birth_year_to, quota)
		VALUES (1, 1, 'U11', 'classical', 2016, 2018, 40)`)
	exec(t, db, `INSERT INTO boards (id, season_id, level, dance_type)
		VALUES (1, 1, 'U11', 'classical')`)
	exec(t, db, `INSERT INTO registrations (id, event_id, group_id, player_id, user_id, dance_type, institution, status)
		VALUES (1, 1, 1, 1, 1, 'classical', '测试机构', 'active')`)
	exec(t, db, `INSERT INTO orders (id, order_no, registration_id, user_id, amount, status, request_id, paid_at)
		VALUES (1, 'ORD-1', 1, 1, 98000, 'paid', 'req-1', '2027-01-01T10:00:00+08:00')`)
	exec(t, db, `INSERT INTO payments (id, order_id, type, prepay_id, transaction_id, amount, status)
		VALUES (1, 1, 'pay', 'prepay-1', 'txn-1', 98000, 'success')`)
	exec(t, db, `INSERT INTO refund_tickets (id, ticket_no, order_id, reason, applicant, status)
		VALUES (1, 'TK-1', 1, '个人原因', '用户本人', 'pending')`)
	exec(t, db, `INSERT INTO waitlists (id, event_id, group_id, dance_type, name, id_card_hash, phone, status, position_no)
		VALUES (1, 1, 1, 'classical', '李四', 'hash-2', '13800000000', 'waiting', 1)`)
	exec(t, db, `INSERT INTO cs_tickets (id, type, phone, content, related_order_id, related_registration_id, status)
		VALUES (1, 'refund', '13800000000', '要求退款', 1, 1, 'pending')`)
	exec(t, db, `INSERT INTO results (id, event_id, board_id, player_id, institution, award,
		score_a, score_b, score_c, score_total, points)
		VALUES (1, 1, 1, 1, '测试机构', 'champion', 95, 96, 97, 288, 100)`)
	exec(t, db, `INSERT INTO player_points (id, season_id, board_id, player_id, total_points, used_event_ids, "rank", computed_at)
		VALUES (1, 1, 1, 1, 100, '[1]', 1, '2027-01-02T00:00:00+08:00')`)
	exec(t, db, `INSERT INTO institution_points (id, season_id, institution_name, total_points, player_count, "rank", computed_at)
		VALUES (1, 1, '测试机构', 100, 1, 1, '2027-01-02T00:00:00+08:00')`)
	exec(t, db, `INSERT INTO final_qualifications (id, season_id, board_id, player_id, source)
		VALUES (1, 1, 1, 1, 'auto_direct')`)
	exec(t, db, `INSERT INTO prizes (id, season_id, event_id, board_id, player_id, award, amount)
		VALUES (1, 1, 1, 1, 1, 'champion', 200000)`)
	exec(t, db, `INSERT INTO admin_logs (id, admin_id, action, target_type, target_id, detail)
		VALUES (1, 1, 'refund.approve', 'order', 1, '{"amount":98000}')`)
}

// tableColumns 返回表的列名集合。
func tableColumns(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("table_info %s: %v", table, err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info %s: %v", table, err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table_info %s rows: %v", table, err)
	}
	return cols
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
