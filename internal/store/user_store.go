package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/meetmonth/dance-elite/internal/model"
)

// UserStore 是 users 表的数据访问层。
type UserStore struct{ db *DB }

// NewUserStore 构造用户 store。
func NewUserStore(db *DB) *UserStore { return &UserStore{db: db} }

const userColumns = `id, openid, unionid, nickname, avatar_url, phone, status,
	last_login_at, created_at, updated_at`

// LoginParams 是微信静默登录时写入/更新的用户字段。
// 空字符串表示微信未提供该字段，不清空库中已有值。
type LoginParams struct {
	OpenID    string
	UnionID   string
	Nickname  string
	AvatarURL string
	Now       string
}

// UpsertLogin 在事务内按 openid 创建或更新用户，并刷新 last_login_at。
//
// openid 唯一；返回 created=true 表示本次新建。nickname/avatar_url/unionid
// 仅在本次提供非空值时覆盖，避免微信未返回字段时清空已有资料。
// 读-改-写在同一事务内完成（SQLite 单连接串行化），并发登录不会产生重复用户。
func (s *UserStore) UpsertLogin(ctx context.Context, tx *sql.Tx, p LoginParams) (*model.User, bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE openid = ?`, p.OpenID).Scan(&id)

	created := false
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, insertErr := tx.ExecContext(ctx, `
			INSERT INTO users (openid, unionid, nickname, avatar_url, last_login_at)
			VALUES (?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?)`,
			p.OpenID, p.UnionID, p.Nickname, p.AvatarURL, p.Now)
		if insertErr != nil {
			return nil, false, fmt.Errorf("insert user %q: %w", p.OpenID, insertErr)
		}
		id, err = res.LastInsertId()
		if err != nil {
			return nil, false, fmt.Errorf("last insert id of user %q: %w", p.OpenID, err)
		}
		created = true
	case err != nil:
		return nil, false, fmt.Errorf("query user %q: %w", p.OpenID, err)
	default:
		if _, err := tx.ExecContext(ctx, `
			UPDATE users
			SET unionid    = COALESCE(NULLIF(?, ''), unionid),
			    nickname   = COALESCE(NULLIF(?, ''), nickname),
			    avatar_url = COALESCE(NULLIF(?, ''), avatar_url),
			    last_login_at = ?
			WHERE id = ?`,
			p.UnionID, p.Nickname, p.AvatarURL, p.Now, id); err != nil {
			return nil, false, fmt.Errorf("update user %d login: %w", id, err)
		}
	}

	user, err := scanUser(tx.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = ?`, id))
	if err != nil {
		return nil, false, fmt.Errorf("reload user %d: %w", id, err)
	}
	return user, created, nil
}

// UpdatePhone 在事务内覆盖用户绑定手机号（明文存储）。
func (s *UserStore) UpdatePhone(ctx context.Context, tx *sql.Tx, userID int64, phone string) error {
	res, err := tx.ExecContext(ctx, `UPDATE users SET phone = ? WHERE id = ?`, phone, userID)
	if err != nil {
		return fmt.Errorf("update phone of user %d: %w", userID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of user %d phone update: %w", userID, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DefaultPlayerNo 返回用户最近一条报名对应的选手编号；没有报名记录时返回 nil。
//
// users 与 players 之间只有报名单作为关联，因此“默认选手”取最近一条
// registration 的 player（含已取消/退赛记录，作为用户的历史身份展示）。
func (s *UserStore) DefaultPlayerNo(ctx context.Context, userID int64) (*string, error) {
	var playerNo string
	err := s.db.sql.QueryRowContext(ctx, `
		SELECT p.player_no
		FROM registrations r
		JOIN players p ON p.id = r.player_id
		WHERE r.user_id = ?
		ORDER BY r.id DESC
		LIMIT 1`, userID).Scan(&playerNo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query default player_no of user %d: %w", userID, err)
	}
	return &playerNo, nil
}

// rowScanner 是 *sql.Row 与 *sql.Rows 的公共 Scan 能力。
type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*model.User, error) {
	var u model.User
	if err := row.Scan(&u.ID, &u.OpenID, &u.UnionID, &u.Nickname, &u.AvatarURL,
		&u.Phone, &u.Status, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}
