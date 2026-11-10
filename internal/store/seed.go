package store

import (
	"context"
	"database/sql"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// DefaultAdminUsername 是初始超管账号的用户名。
const DefaultAdminUsername = "admin"

// SeedOptions 控制启动时写入的动态种子数据。
//
// 静态配置（settings 默认模板）由迁移 0007 写入；需要运行时计算的种子
// （bcrypt 密码哈希）在此处理。
type SeedOptions struct {
	// AdminUsername 初始超管用户名，为空时使用 DefaultAdminUsername。
	AdminUsername string
	// AdminPassword 初始超管密码；为空时跳过超管种子（由调用方记录警告）。
	AdminPassword string
}

// Seed 写入启动所需种子数据，可重复执行。
//
// 超管账号仅在不存在时创建：已存在同名账号时不做任何修改（密码以库中为准，
// 避免每次启动用环境变量覆盖已修改的密码）。
func (d *DB) Seed(ctx context.Context, opts SeedOptions) error {
	if opts.AdminPassword == "" {
		return nil
	}
	username := opts.AdminUsername
	if username == "" {
		username = DefaultAdminUsername
	}
	if len(opts.AdminPassword) > 72 {
		return fmt.Errorf("seed admin: password exceeds bcrypt limit of 72 bytes")
	}

	exists, err := adminExists(ctx, d.sql, username)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(opts.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("seed admin: hash password: %w", err)
	}
	if err := d.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO admins (username, password_hash, role) VALUES (?, ?, 'super_admin')`,
			username, string(hash)); err != nil {
			return fmt.Errorf("seed admin: insert %q: %w", username, err)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func adminExists(ctx context.Context, db *sql.DB, username string) (bool, error) {
	var exists int
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM admins WHERE username = ?)`, username).Scan(&exists); err != nil {
		return false, fmt.Errorf("seed admin: query %q: %w", username, err)
	}
	return exists == 1, nil
}
