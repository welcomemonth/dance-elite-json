package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// newAdminTestStore 打开内存库、跑完迁移并写入一个超管账号。
func newAdminTestStore(t *testing.T) (*AdminStore, *DB) {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := Migrate(context.Background(), db.SQL()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Seed(context.Background(), SeedOptions{
		AdminUsername: "admin", AdminPassword: "Init@12345",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return NewAdminStore(db), db
}

func TestAdminByUsername(t *testing.T) {
	admins, _ := newAdminTestStore(t)

	admin, err := admins.ByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatalf("ByUsername: %v", err)
	}
	if admin.ID == 0 || admin.Username != "admin" || admin.Role != "super_admin" {
		t.Fatalf("admin = %+v", admin)
	}
	if admin.Name == nil || *admin.Name != DefaultAdminName {
		t.Fatalf("name = %v, want %q", admin.Name, DefaultAdminName)
	}
	if admin.Status != "active" || admin.LastLoginAt != nil {
		t.Fatalf("status/last_login_at = %q/%v, want active/nil", admin.Status, admin.LastLoginAt)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("Init@12345")); err != nil {
		t.Fatal("password hash does not match seeded password")
	}

	if _, err := admins.ByUsername(context.Background(), "nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ByUsername(nobody) error = %v, want ErrNotFound", err)
	}
}

func TestAdminTouchLogin(t *testing.T) {
	admins, db := newAdminTestStore(t)
	ctx := context.Background()

	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return admins.TouchLogin(ctx, tx, 1, "2026-11-10T12:00:00+08:00")
	})
	if err != nil {
		t.Fatalf("TouchLogin: %v", err)
	}

	admin, err := admins.Get(ctx, 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if admin.LastLoginAt == nil || *admin.LastLoginAt != "2026-11-10T12:00:00+08:00" {
		t.Fatalf("last_login_at = %v", admin.LastLoginAt)
	}

	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		return admins.TouchLogin(ctx, tx, 999, "2026-11-10T12:00:00+08:00")
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("TouchLogin(999) error = %v, want ErrNotFound", err)
	}
}

func TestAdminUpdatePassword(t *testing.T) {
	admins, db := newAdminTestStore(t)
	ctx := context.Background()

	hash, err := bcrypt.GenerateFromPassword([]byte("NewPass#888"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		return admins.UpdatePassword(ctx, tx, 1, string(hash))
	})
	if err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}

	admin, err := admins.ByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("ByUsername: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("NewPass#888")); err != nil {
		t.Fatal("stored hash does not match new password")
	}

	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		return admins.UpdatePassword(ctx, tx, 999, string(hash))
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdatePassword(999) error = %v, want ErrNotFound", err)
	}
}

func TestAdminInsertLog(t *testing.T) {
	admins, db := newAdminTestStore(t)
	ctx := context.Background()

	adminID, targetID := int64(1), int64(1)
	detail := []byte(`{"username":"admin","result":"success"}`)
	err := db.WithTx(ctx, func(tx *sql.Tx) error {
		return admins.InsertLog(ctx, tx, AdminLogParams{
			AdminID:    &adminID,
			Action:     "admin.login",
			TargetType: strPtrStore("admin"),
			TargetID:   &targetID,
			Detail:     detail,
		})
	})
	if err != nil {
		t.Fatalf("InsertLog: %v", err)
	}

	// admin_id 为 nil 的系统动作也应可写入。
	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		return admins.InsertLog(ctx, tx, AdminLogParams{
			AdminID: nil,
			Action:  "admin.login_locked",
			Detail:  []byte(`{"username":"admin"}`),
		})
	})
	if err != nil {
		t.Fatalf("InsertLog(system): %v", err)
	}

	var rows int
	if err := db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM admin_logs WHERE action IN ('admin.login', 'admin.login_locked')`).Scan(&rows); err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if rows != 2 {
		t.Fatalf("log rows = %d, want 2", rows)
	}

	// detail 必须满足 json_valid 约束：非 JSON 文本被拒绝。
	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		return admins.InsertLog(ctx, tx, AdminLogParams{Action: "bad", Detail: []byte("not-json")})
	})
	if err == nil {
		t.Fatal("InsertLog with non-JSON detail should fail")
	}

	// action 为空是调用方错误，store 层直接拒绝。
	err = db.WithTx(ctx, func(tx *sql.Tx) error {
		return admins.InsertLog(ctx, tx, AdminLogParams{Action: ""})
	})
	if err == nil {
		t.Fatal("InsertLog with empty action should fail")
	}
}

func strPtrStore(s string) *string { return &s }
