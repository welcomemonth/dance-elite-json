package store

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
)

// migrationsBefore 返回版本号小于 maxVersion 的迁移子集（模拟升级前的库）。
func migrationsBefore(t *testing.T, maxVersion int) fstest.MapFS {
	t.Helper()
	entries, err := fs.ReadDir(migrationsFS, migrateDir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	fsys := fstest.MapFS{}
	for _, e := range entries {
		version, err := parseVersion(e.Name())
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		if version >= maxVersion {
			continue
		}
		data, err := fs.ReadFile(migrationsFS, migrateDir+"/"+e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		fsys[migrateDir+"/"+e.Name()] = &fstest.MapFile{Data: data}
	}
	return fsys
}

// TestMigrate0013PreservesData 验证 players 重建迁移在已有数据与引用关系下可用：
// 数据保留、外键仍干净、新证件类型/国籍与 orders.source 生效。
func TestMigrate0013PreservesData(t *testing.T) {
	db := openMemory(t)
	ctx := context.Background()

	if err := migrateFS(ctx, db.SQL(), migrationsBefore(t, 13)); err != nil {
		t.Fatalf("migrate to v12: %v", err)
	}
	insertFixtures(t, db.SQL())

	if err := Migrate(ctx, db.SQL()); err != nil {
		t.Fatalf("migrate v13: %v", err)
	}

	var name, idType string
	if err := db.SQL().QueryRowContext(ctx,
		`SELECT name, id_type FROM players WHERE id = 1`).Scan(&name, &idType); err != nil {
		t.Fatalf("query preserved player: %v", err)
	}
	if name != "张三" || idType != "id_card" {
		t.Fatalf("player = %q/%q, want 张三/id_card", name, idType)
	}

	rows, err := db.SQL().QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key violation after players rebuild")
	}

	for typ, playerNo := range map[string]string{
		"passport": "Psspt001", "hk_macau": "Phkm0001", "other": "Poth0001",
	} {
		if _, err := db.SQL().ExecContext(ctx,
			`INSERT INTO players (player_no, name, id_type) VALUES (?, ?, ?)`,
			playerNo, "外籍选手", typ); err != nil {
			t.Fatalf("insert id_type %s: %v", typ, err)
		}
	}
	if _, err := db.SQL().ExecContext(ctx,
		`INSERT INTO players (player_no, name, id_type) VALUES ('BadType1', '非法', 'bogus')`); err == nil {
		t.Fatal("expected CHECK violation for bogus id_type")
	}

	if _, err := db.SQL().ExecContext(ctx,
		`UPDATE players SET nationality = '美国' WHERE id = 1`); err != nil {
		t.Fatalf("update nationality: %v", err)
	}

	var source string
	if err := db.SQL().QueryRowContext(ctx,
		`SELECT source FROM orders WHERE id = 1`).Scan(&source); err != nil {
		t.Fatalf("query order source: %v", err)
	}
	if source != "miniapp" {
		t.Fatalf("default order source = %q, want miniapp", source)
	}
	if _, err := db.SQL().ExecContext(ctx,
		`UPDATE orders SET source = 'manual' WHERE id = 1`); err != nil {
		t.Fatalf("update order source manual: %v", err)
	}
}
