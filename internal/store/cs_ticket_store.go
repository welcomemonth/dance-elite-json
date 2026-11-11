package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
)

// CSTicketStore 是客服工单（cs_tickets 表）的数据访问层（开发计划 T16）。
type CSTicketStore struct{ db *DB }

// NewCSTicketStore 构造客服工单 store。
func NewCSTicketStore(db *DB) *CSTicketStore { return &CSTicketStore{db: db} }

const csTicketColumns = `id, type, user_name, phone, content, related_order_id,
	related_registration_id, status, remark, created_by, created_at, updated_at`

// CSTicketParams 是登记工单的字段；RelatedOrderID / RelatedRegistrationID 可空。
type CSTicketParams struct {
	Type                  string
	UserName              *string
	Phone                 string
	Content               string
	RelatedOrderID        *int64
	RelatedRegistrationID *int64
	CreatedBy             int64
}

// CreateTx 在事务内插入一条 pending 工单，返回工单 ID。
func (s *CSTicketStore) CreateTx(ctx context.Context, tx *sql.Tx, p CSTicketParams) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO cs_tickets (type, user_name, phone, content, related_order_id,
			related_registration_id, status, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Type, p.UserName, p.Phone, p.Content, p.RelatedOrderID,
		p.RelatedRegistrationID, model.CSTicketStatusPending, p.CreatedBy)
	if err != nil {
		return 0, fmt.Errorf("insert cs ticket: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id of cs ticket: %w", err)
	}
	return id, nil
}

// Get 按 id 查询工单（非事务读）。不存在返回 ErrNotFound。
func (s *CSTicketStore) Get(ctx context.Context, id int64) (*model.CSTicket, error) {
	t, err := scanCSTicket(s.db.sql.QueryRowContext(ctx,
		`SELECT `+csTicketColumns+` FROM cs_tickets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query cs ticket %d: %w", id, err)
	}
	return t, nil
}

// GetTx 在事务内按 id 查询工单。
func (s *CSTicketStore) GetTx(ctx context.Context, tx *sql.Tx, id int64) (*model.CSTicket, error) {
	t, err := scanCSTicket(tx.QueryRowContext(ctx,
		`SELECT `+csTicketColumns+` FROM cs_tickets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query cs ticket %d in tx: %w", id, err)
	}
	return t, nil
}

// UpdateStatusTx 条件流转工单：仅当当前状态为 fromStatus 时更新（防并发覆盖），
// remark 为空时保留原备注。条件未命中返回 ErrConflict。
func (s *CSTicketStore) UpdateStatusTx(ctx context.Context, tx *sql.Tx, id int64, fromStatus, toStatus string, remark *string) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE cs_tickets SET status = ?, remark = COALESCE(?, remark)
		WHERE id = ? AND status = ?`, toStatus, remark, id, fromStatus)
	if err != nil {
		return fmt.Errorf("update cs ticket %d status: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of cs ticket %d update: %w", id, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// AdminCSTicketFilter 是后台工单列表筛选；CreatedBy > 0 时只看该客服登记的工单。
type AdminCSTicketFilter struct {
	Type      string
	Status    string
	CreatedBy int64
	Page      int
	PageSize  int
}

// AdminCSTicketRow 是列表行（关联订单号与登记人用户名）。
type AdminCSTicketRow struct {
	TicketID              int64
	Type                  string
	UserName              *string
	Phone                 string
	Content               string
	RelatedOrderNo        *string
	RelatedRegistrationID *int64
	Status                string
	Remark                *string
	CreatedBy             *int64
	CreatedByName         *string
	CreatedAt             string
	UpdatedAt             string
}

// ListAdmin 返回后台工单分页列表（type/status/created_by 筛选），新的在前。
func (s *CSTicketStore) ListAdmin(ctx context.Context, f AdminCSTicketFilter) ([]AdminCSTicketRow, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	if f.Type != "" {
		where = append(where, "t.type = ?")
		args = append(args, f.Type)
	}
	if f.Status != "" {
		where = append(where, "t.status = ?")
		args = append(args, f.Status)
	}
	if f.CreatedBy > 0 {
		where = append(where, "t.created_by = ?")
		args = append(args, f.CreatedBy)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := s.db.sql.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM cs_tickets t WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count admin cs tickets: %w", err)
	}
	if total == 0 {
		return []AdminCSTicketRow{}, 0, nil
	}

	rows, err := s.db.sql.QueryContext(ctx, `
		SELECT t.id, t.type, t.user_name, t.phone, t.content, o.order_no,
		       t.related_registration_id, t.status, t.remark, t.created_by,
		       a.username, t.created_at, t.updated_at
		FROM cs_tickets t
		LEFT JOIN orders o ON o.id = t.related_order_id
		LEFT JOIN admins a ON a.id = t.created_by
		WHERE `+cond+`
		ORDER BY t.created_at DESC, t.id DESC
		LIMIT ? OFFSET ?`,
		append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin cs tickets: %w", err)
	}
	defer rows.Close()

	list := make([]AdminCSTicketRow, 0, f.PageSize)
	for rows.Next() {
		var r AdminCSTicketRow
		if err := rows.Scan(&r.TicketID, &r.Type, &r.UserName, &r.Phone, &r.Content,
			&r.RelatedOrderNo, &r.RelatedRegistrationID, &r.Status, &r.Remark,
			&r.CreatedBy, &r.CreatedByName, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan admin cs ticket: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate admin cs tickets: %w", err)
	}
	return list, total, nil
}

func scanCSTicket(row rowScanner) (*model.CSTicket, error) {
	var t model.CSTicket
	if err := row.Scan(&t.ID, &t.Type, &t.UserName, &t.Phone, &t.Content,
		&t.RelatedOrderID, &t.RelatedRegistrationID, &t.Status, &t.Remark,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}
