package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
)

// RefundTicketStore 是退款工单（refund_tickets 表）的数据访问层（T15）。
type RefundTicketStore struct{ db *DB }

// NewRefundTicketStore 构造退款工单 store。
func NewRefundTicketStore(db *DB) *RefundTicketStore { return &RefundTicketStore{db: db} }

const refundTicketColumns = `id, ticket_no, order_id, reason, applicant, status,
	handler_id, reject_reason, handled_at, created_by, created_at, updated_at`

// Get 按 id 查询退款工单。不存在返回 ErrNotFound。
func (s *RefundTicketStore) Get(ctx context.Context, id int64) (*model.RefundTicket, error) {
	return s.get(ctx, s.db.sql, id)
}

// GetTx 在事务内按 id 查询退款工单（处理/回调复核状态）。
func (s *RefundTicketStore) GetTx(ctx context.Context, tx *sql.Tx, id int64) (*model.RefundTicket, error) {
	return s.get(ctx, tx, id)
}

func (s *RefundTicketStore) get(ctx context.Context, q queryer, id int64) (*model.RefundTicket, error) {
	ticket, err := scanRefundTicket(q.QueryRowContext(ctx,
		`SELECT `+refundTicketColumns+` FROM refund_tickets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query refund ticket %d: %w", id, err)
	}
	return ticket, nil
}

// GetByTicketNoTx 在事务内按工单号查询（退款回调以 out_refund_no=ticket_no 关联）。
func (s *RefundTicketStore) GetByTicketNoTx(ctx context.Context, tx *sql.Tx, ticketNo string) (*model.RefundTicket, error) {
	ticket, err := scanRefundTicket(tx.QueryRowContext(ctx,
		`SELECT `+refundTicketColumns+` FROM refund_tickets WHERE ticket_no = ?`, ticketNo))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query refund ticket %q: %w", ticketNo, err)
	}
	return ticket, nil
}

// RefundTicketParams 是创建退款工单的字段。
type RefundTicketParams struct {
	TicketNo  string
	OrderID   int64
	Reason    string
	Applicant *string
	Status    string
	CreatedBy *int64
}

// CreateTx 在事务内创建退款工单，返回新 id。
func (s *RefundTicketStore) CreateTx(ctx context.Context, tx *sql.Tx, p RefundTicketParams) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO refund_tickets (ticket_no, order_id, reason, applicant, status, created_by)
		VALUES (?, ?, ?, ?, ?, ?)`,
		p.TicketNo, p.OrderID, p.Reason, p.Applicant, p.Status, p.CreatedBy)
	if err != nil {
		return 0, fmt.Errorf("insert refund ticket of order %d: %w", p.OrderID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id of refund ticket of order %d: %w", p.OrderID, err)
	}
	return id, nil
}

// HasActiveByOrderTx 在事务内判断订单是否已有进行中的退款工单
// （RefundTicketActiveStatuses；驳回后可重新申请）。
func (s *RefundTicketStore) HasActiveByOrderTx(ctx context.Context, tx *sql.Tx, orderID int64) (bool, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(model.RefundTicketActiveStatuses)), ",")
	args := make([]any, 0, len(model.RefundTicketActiveStatuses)+1)
	args = append(args, orderID)
	for _, status := range model.RefundTicketActiveStatuses {
		args = append(args, status)
	}
	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM refund_tickets WHERE order_id = ? AND status IN (`+placeholders+`)`,
		args...).Scan(&n); err != nil {
		return false, fmt.Errorf("count active refund tickets of order %d: %w", orderID, err)
	}
	return n > 0, nil
}

func scanRefundTicket(row rowScanner) (*model.RefundTicket, error) {
	var t model.RefundTicket
	if err := row.Scan(&t.ID, &t.TicketNo, &t.OrderID, &t.Reason, &t.Applicant, &t.Status,
		&t.HandlerID, &t.RejectReason, &t.HandledAt, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}
