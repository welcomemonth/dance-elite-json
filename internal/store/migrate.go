package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// migrationsFS 嵌入全部迁移 SQL，保证单二进制部署（Docker）时无需外挂文件。
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrateDir 是迁移 SQL 文件在嵌入 FS 中的目录前缀。
const migrateDir = "migrations"

// Migrate 按文件名版本顺序执行 migrations/*.sql 中尚未应用的迁移。
//
// 版本号取自文件名下划线前的数字前缀（如 0001_init.sql -> 1）。
// 每个迁移在独立事务中执行，成功后在 schema_migrations 表记录版本，
// 重复调用是幂等的：已记录的版本直接跳过。
func Migrate(ctx context.Context, db *sql.DB) error {
	return migrateFS(ctx, db, migrationsFS)
}

// migrateFS 以任意 fs.FS 为迁移来源，便于测试注入内存文件系统。
func migrateFS(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now', 'localtime'))
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}

	files, err := migrationFiles(fsys)
	if err != nil {
		return err
	}

	for _, f := range files {
		if applied[f.version] {
			continue
		}
		if err := applyMigration(ctx, db, fsys, f); err != nil {
			return err
		}
	}
	return nil
}

// migrationFile 表示一个待执行的迁移文件。
type migrationFile struct {
	version int
	name    string
}

// migrationFiles 读取迁移目录下全部 .sql 文件并按版本号升序返回。
func migrationFiles(fsys fs.FS) ([]migrationFile, error) {
	entries, err := fs.ReadDir(fsys, migrateDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var files []migrationFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version, err := parseVersion(e.Name())
		if err != nil {
			return nil, err
		}
		files = append(files, migrationFile{version: version, name: e.Name()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].version < files[j].version })
	return files, nil
}

// parseVersion 解析 "0001_xxx.sql" 的文件名前缀为版本号。
func parseVersion(name string) (int, error) {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	idx := strings.Index(stem, "_")
	if idx <= 0 {
		return 0, fmt.Errorf("migration filename %q missing version prefix (expected NNNN_name.sql)", name)
	}
	v, err := strconv.Atoi(stem[:idx])
	if err != nil {
		return 0, fmt.Errorf("migration filename %q has invalid version: %w", name, err)
	}
	return v, nil
}

// appliedVersions 返回已应用的迁移版本集合。
func appliedVersions(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// applyMigration 在单个事务中执行一个迁移文件并登记版本。
//
// SQLite 修改列约束只能重建表（CREATE 新表 → 复制 → DROP 旧表 → RENAME），
// 而 DROP 被其他表引用的父表需要关闭外键校验；PRAGMA foreign_keys 在事务内
// 不生效，因此在事务外关闭、迁移完成后恢复（连接池为单连接，状态不串扰）。
func applyMigration(ctx context.Context, db *sql.DB, fsys fs.FS, f migrationFile) error {
	body, err := fs.ReadFile(fsys, filepath.ToSlash(filepath.Join(migrateDir, f.name)))
	if err != nil {
		return fmt.Errorf("read migration %s: %w", f.name, err)
	}

	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disable foreign keys for migration %s: %w", f.name, err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), `PRAGMA foreign_keys = ON`)
	}()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", f.name, err)
	}
	defer func() { _ = tx.Rollback() }()

	// 一个文件可含多条语句， modernc 驱动顺序执行。
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return fmt.Errorf("apply migration %s: %w", f.name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version) VALUES (?)`, f.version); err != nil {
		return fmt.Errorf("record migration %s: %w", f.name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", f.name, err)
	}
	return nil
}
