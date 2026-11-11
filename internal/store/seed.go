package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// DefaultAdminUsername 是初始超管账号的用户名。
const DefaultAdminUsername = "admin"

// DefaultAdminName 是初始超管的默认显示名（真实姓名由后续账号管理维护）。
const DefaultAdminName = "超级管理员"

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
// 超管账号仅在不存在时创建：已存在同名账号时密码与角色以库中为准，
// 避免每次启动用环境变量覆盖已修改的密码；仅在 name 为空时补默认显示名
// （兼容 0008 迁移前创建的历史账号）。
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

	id, name, err := adminLookup(ctx, d.sql, username)
	if err != nil {
		return err
	}
	if id != 0 {
		if name != nil {
			return nil
		}
		return d.WithTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx,
				`UPDATE admins SET name = ? WHERE id = ? AND name IS NULL`,
				DefaultAdminName, id); err != nil {
				return fmt.Errorf("seed admin: backfill name of %q: %w", username, err)
			}
			return nil
		})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(opts.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("seed admin: hash password: %w", err)
	}
	if err := d.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO admins (username, name, password_hash, role) VALUES (?, ?, ?, 'super_admin')`,
			username, DefaultAdminName, string(hash)); err != nil {
			return fmt.Errorf("seed admin: insert %q: %w", username, err)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

// adminLookup 按用户名返回管理员 ID 与姓名；不存在时返回 id=0。
func adminLookup(ctx context.Context, db *sql.DB, username string) (id int64, name *string, err error) {
	err = db.QueryRowContext(ctx,
		`SELECT id, name FROM admins WHERE username = ?`, username).Scan(&id, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, fmt.Errorf("seed admin: query %q: %w", username, err)
	}
	return id, name, nil
}
