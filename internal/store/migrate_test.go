package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"testing/fstest"
)

var errSentinel = errors.New("sentinel")

func openMemory(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestMigrateIdempotent 重复执行迁移不得报错且只应用一次。
func TestMigrateIdempotent(t *testing.T) {
	db := openMemory(t)
	ctx := context.Background()

	if err := Migrate(ctx, db.SQL()); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	var count int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if count != 1 {
		t.Fatalf("applied versions = %d, want 1 (placeholder migration)", count)
	}

	// 第二次执行：全部跳过，版本数不变
	if err := Migrate(ctx, db.SQL()); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count versions after re-run: %v", err)
	}
	if count != 1 {
		t.Fatalf("applied versions after re-run = %d, want still 1", count)
	}
}

// TestMigrateOrderAndSkip 验证按版本序执行、已应用版本跳过。
func TestMigrateOrderAndSkip(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/0002_add_bar.sql": &fstest.MapFile{Data: []byte(
			`CREATE TABLE bar (id INTEGER PRIMARY KEY);`)},
		"migrations/0001_add_foo.sql": &fstest.MapFile{Data: []byte(
			`CREATE TABLE foo (id INTEGER PRIMARY KEY);`)},
	}
	db := openMemory(t)
	ctx := context.Background()

	if err := migrateFS(ctx, db.SQL(), fsys); err != nil {
		t.Fatalf("migrateFS: %v", err)
	}

	var fooApplied, barApplied int
	err := db.SQL().QueryRow(`
		SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = 1),
		       EXISTS(SELECT 1 FROM schema_migrations WHERE version = 2)`).Scan(&fooApplied, &barApplied)
	if err != nil {
		t.Fatalf("query versions: %v", err)
	}
	if fooApplied != 1 || barApplied != 1 {
		t.Fatalf("versions recorded incorrectly: foo=%d bar=%d", fooApplied, barApplied)
	}

	// 两表都已创建（顺序执行的副作用校验）
	for _, table := range []string{"foo", "bar"} {
		var name string
		if err := db.SQL().QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name); err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}

	// 重复执行无副作用
	if err := migrateFS(ctx, db.SQL(), fsys); err != nil {
		t.Fatalf("re-run migrateFS: %v", err)
	}
	var count int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("versions = %d, want 2", count)
	}
}

// TestMigrateFailureRollsBack 失败的迁移不得留下半个版本记录。
func TestMigrateFailureRollsBack(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/0001_bad.sql": &fstest.MapFile{Data: []byte(
			`CREATE TABLE half (id INTEGER); THIS IS NOT SQL;`)},
	}
	db := openMemory(t)
	if err := migrateFS(context.Background(), db.SQL(), fsys); err == nil {
		t.Fatal("expected migration error, got nil")
	}
	var count int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("failed migration left %d version records, want 0", count)
	}
}

// TestMigrateBadFilename 缺少版本前缀的文件名必须报错。
func TestMigrateBadFilename(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/nover.sql": &fstest.MapFile{Data: []byte(`SELECT 1;`)},
	}
	db := openMemory(t)
	if err := migrateFS(context.Background(), db.SQL(), fsys); err == nil {
		t.Fatal("expected filename error, got nil")
	}
}

// TestOpenAppliesPragmas 打开的文件库应具备 WAL 与外键设置。
func TestOpenAppliesPragmas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	var journalMode, foreignKeys string
	if err := db.SQL().QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if err := db.SQL().QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
	if foreignKeys != "1" {
		t.Fatalf("foreign_keys = %q, want 1", foreignKeys)
	}
}

// TestWithTxCommitAndRollback 事务提交/回滚行为。
func TestWithTxCommitAndRollback(t *testing.T) {
	db := openMemory(t)
	ctx := context.Background()

	if _, err := db.SQL().ExecContext(ctx, `CREATE TABLE t (id INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	// 提交：数据落库
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO t (id) VALUES (1)`)
		return err
	})
	if err != nil {
		t.Fatalf("commit tx: %v", err)
	}
	var n int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM t`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows = %d, want 1 after commit", n)
	}

	// 回滚：fn 返回错误时数据不落库，且原样上抛
	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		_, _ = tx.ExecContext(ctx, `INSERT INTO t (id) VALUES (2)`)
		return errSentinel
	})
	if !errors.Is(err, errSentinel) {
		t.Fatalf("WithTx error = %v, want sentinel", err)
	}
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM t`).Scan(&n); err != nil {
		t.Fatalf("count after rollback: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows = %d, want 1 after rollback", n)
	}
}
