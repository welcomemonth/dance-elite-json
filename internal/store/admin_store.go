package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/meetmonth/dance-elite/internal/model"
)

// AdminStore 是 admins + admin_logs 表的数据访问层。
type AdminStore struct{ db *DB }

// NewAdminStore 构造管理员 store。
func NewAdminStore(db *DB) *AdminStore { return &AdminStore{db: db} }

const adminColumns = `id, username, name, password_hash, role, status,
	last_login_at, created_at, updated_at`

// ByUsername 按用户名读取管理员（含密码哈希）。
//
// 密码哈希仅限服务端 bcrypt 校验使用；不存在返回 ErrNotFound。
func (s *AdminStore) ByUsername(ctx context.Context, username string) (*model.Admin, error) {
	admin, err := scanAdmin(s.db.sql.QueryRowContext(ctx,
		`SELECT `+adminColumns+` FROM admins WHERE username = ?`, username))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query admin %q: %w", username, err)
	}
	return admin, nil
}

// Get 按 ID 读取管理员（含密码哈希，供改密等内部场景使用）。
func (s *AdminStore) Get(ctx context.Context, id int64) (*model.Admin, error) {
	admin, err := scanAdmin(s.db.sql.QueryRowContext(ctx,
		`SELECT `+adminColumns+` FROM admins WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query admin %d: %w", id, err)
	}
	return admin, nil
}

// TouchLogin 在事务内刷新管理员最后登录时间。
func (s *AdminStore) TouchLogin(ctx context.Context, tx *sql.Tx, adminID int64, now string) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE admins SET last_login_at = ? WHERE id = ?`, now, adminID)
	if err != nil {
		return fmt.Errorf("touch login of admin %d: %w", adminID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of admin %d login touch: %w", adminID, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdatePassword 在事务内覆盖管理员密码哈希。
func (s *AdminStore) UpdatePassword(ctx context.Context, tx *sql.Tx, adminID int64, passwordHash string) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE admins SET password_hash = ? WHERE id = ?`, passwordHash, adminID)
	if err != nil {
		return fmt.Errorf("update password of admin %d: %w", adminID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of admin %d password update: %w", adminID, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminLogParams 是一条操作日志（admin_logs）的写入参数。
// Detail 为 JSON 文本（如 {"username":"admin"}），nil 表示无详情。
type AdminLogParams struct {
	// AdminID 可为 nil：admin_id 为空表示系统动作（如登录锁定）。
	AdminID    *int64
	Action     string
	TargetType *string
	TargetID   *int64
	Detail     []byte
}

// InsertLog 在事务内追加一条 admin_logs。
//
// admin_logs 只追加不修改（schema 注释：敏感操作留痕）；后续所有敏感操作
// （退款、成绩纠错、明文查看等）都经由此助手写入。
func (s *AdminStore) InsertLog(ctx context.Context, tx *sql.Tx, p AdminLogParams) error {
	if p.Action == "" {
		return errors.New("insert admin log: action is required")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO admin_logs (admin_id, action, target_type, target_id, detail)
		VALUES (?, ?, ?, ?, ?)`,
		p.AdminID, p.Action, p.TargetType, p.TargetID, p.Detail); err != nil {
		return fmt.Errorf("insert admin log %q: %w", p.Action, err)
	}
	return nil
}

func scanAdmin(row rowScanner) (*model.Admin, error) {
	var a model.Admin
	if err := row.Scan(&a.ID, &a.Username, &a.Name, &a.PasswordHash, &a.Role, &a.Status,
		&a.LastLoginAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}
