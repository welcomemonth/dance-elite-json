package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/meetmonth/dance-elite/internal/model"
)

// SeasonStore 是赛季（seasons 表）的数据访问层。
type SeasonStore struct{ db *DB }

// NewSeasonStore 构造赛季 store。
func NewSeasonStore(db *DB) *SeasonStore { return &SeasonStore{db: db} }

const seasonColumns = `id, name, start_date, end_date, status, group_rules, archived_at,
	created_at, updated_at`

// SeasonParams 是创建赛季的字段。GroupRules 从系统设置复制（service 层负责取模板）。
type SeasonParams struct {
	Name       string
	StartDate  *string
	EndDate    *string
	GroupRules string
}

// SeasonPatch 是编辑赛季的字段；nil 指针表示不修改该列。
type SeasonPatch struct {
	Name       *string
	StartDate  *string
	EndDate    *string
	Status     *string
	ArchivedAt *string // 转入 archived 时由 service 传入当前时间
}

// List 返回全部赛季，按 id 倒序（最近的在前）。
func (s *SeasonStore) List(ctx context.Context) ([]model.Season, error) {
	rows, err := s.db.sql.QueryContext(ctx, `SELECT `+seasonColumns+` FROM seasons ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query seasons: %w", err)
	}
	defer rows.Close()

	var list []model.Season
	for rows.Next() {
		season, err := scanSeason(rows)
		if err != nil {
			return nil, fmt.Errorf("scan season: %w", err)
		}
		list = append(list, *season)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate seasons: %w", err)
	}
	return list, nil
}

// Get 按 id 查询赛季。不存在返回 ErrNotFound。
func (s *SeasonStore) Get(ctx context.Context, id int64) (*model.Season, error) {
	season, err := scanSeason(s.db.sql.QueryRowContext(ctx,
		`SELECT `+seasonColumns+` FROM seasons WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query season %d: %w", id, err)
	}
	return season, nil
}

// HasOtherActive 返回除 excludeID 外是否已有 active 赛季（唯一活跃赛季约束）。
func (s *SeasonStore) HasOtherActive(ctx context.Context, excludeID int64) (bool, error) {
	var n int
	err := s.db.sql.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM seasons WHERE status = 'active' AND id <> ?`, excludeID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("count active seasons: %w", err)
	}
	return n > 0, nil
}

// Insert 在事务内创建赛季（状态恒为 upcoming）。
func (s *SeasonStore) Insert(ctx context.Context, tx *sql.Tx, p SeasonParams) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO seasons (name, start_date, end_date, group_rules)
		VALUES (?, NULLIF(?, ''), NULLIF(?, ''), ?)`,
		p.Name, derefOrEmpty(p.StartDate), derefOrEmpty(p.EndDate), p.GroupRules)
	if err != nil {
		return 0, fmt.Errorf("insert season %q: %w", p.Name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id of season %q: %w", p.Name, err)
	}
	return id, nil
}

// Activate 在事务内把赛季置为 active（条件更新：当前状态必须为 from）。
// 并发/重复激活由部分唯一索引 uq_seasons_single_active 兜底，
// 违反时返回 ErrConflict（service 层映射为 40901）；
// 条件未命中（状态不符）也返回 ErrConflict，不存在返回 ErrNotFound。
func (s *SeasonStore) Activate(ctx context.Context, tx *sql.Tx, id int64, from string) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE seasons SET status = 'active' WHERE id = ? AND status = ?`, id, from)
	if err != nil {
		if IsUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("activate season %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of season %d activate: %w", id, err)
	}
	if n == 0 {
		// 单连接池下事务内经 db.sql 再查会自死锁，存在性检查用同一事务连接完成。
		var cnt int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM seasons WHERE id = ?`, id).Scan(&cnt); err != nil {
			return fmt.Errorf("check existence of season %d: %w", id, err)
		}
		if cnt == 0 {
			return ErrNotFound
		}
		return ErrConflict
	}
	return nil
}

// Update 在事务内按非 nil 字段更新赛季；未提供 Status 时不触碰 archived_at。
func (s *SeasonStore) Update(ctx context.Context, tx *sql.Tx, id int64, patch SeasonPatch) error {
	if patch.Name != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE seasons SET name = ? WHERE id = ?`, *patch.Name, id); err != nil {
			return fmt.Errorf("update season %d name: %w", id, err)
		}
	}
	if patch.StartDate != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE seasons SET start_date = NULLIF(?, '') WHERE id = ?`, *patch.StartDate, id); err != nil {
			return fmt.Errorf("update season %d start_date: %w", id, err)
		}
	}
	if patch.EndDate != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE seasons SET end_date = NULLIF(?, '') WHERE id = ?`, *patch.EndDate, id); err != nil {
			return fmt.Errorf("update season %d end_date: %w", id, err)
		}
	}
	if patch.Status != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE seasons SET status = ? WHERE id = ?`, *patch.Status, id); err != nil {
			return fmt.Errorf("update season %d status: %w", id, err)
		}
	}
	if patch.ArchivedAt != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE seasons SET archived_at = ? WHERE id = ?`, *patch.ArchivedAt, id); err != nil {
			return fmt.Errorf("update season %d archived_at: %w", id, err)
		}
	}
	return nil
}

func derefOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func scanSeason(row rowScanner) (*model.Season, error) {
	var s model.Season
	if err := row.Scan(&s.ID, &s.Name, &s.StartDate, &s.EndDate, &s.Status,
		&s.GroupRules, &s.ArchivedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	return &s, nil
}
