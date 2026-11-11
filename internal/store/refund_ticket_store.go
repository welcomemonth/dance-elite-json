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

// MarkProcessingTx 在事务内认领工单：pending/refund_failed/processing → processing，
// 记录处理人。用于同意退款发起前抢占，防并发重复处理；退回失败后可重试。
func (s *RefundTicketStore) MarkProcessingTx(ctx context.Context, tx *sql.Tx, id, handlerID int64) error {
	return s.transitionTx(ctx, tx, `
		UPDATE refund_tickets SET status = 'processing', handler_id = ?
		WHERE id = ? AND status IN ('pending', 'refund_failed', 'processing')`,
		"mark processing", handlerID, id)
}

// MarkApprovedTx 在事务内把处理中的工单置为已同意（微信退款接口已受理）。
func (s *RefundTicketStore) MarkApprovedTx(ctx context.Context, tx *sql.Tx, id int64, handledAt string) error {
	return s.transitionTx(ctx, tx, `
		UPDATE refund_tickets SET status = 'approved', handled_at = ?
		WHERE id = ? AND status = 'processing'`,
		"mark approved", handledAt, id)
}

// MarkRefundFailedTx 在事务内把工单置为退款失败（接口失败/关闭回调），可重试。
// handlerID 可为 nil（回调路径无操作人）。
func (s *RefundTicketStore) MarkRefundFailedTx(ctx context.Context, tx *sql.Tx, id int64, handlerID *int64, handledAt string) error {
	return s.transitionTx(ctx, tx, `
		UPDATE refund_tickets SET status = 'refund_failed', handler_id = COALESCE(?, handler_id), handled_at = ?
		WHERE id = ? AND status IN ('pending', 'processing', 'approved')`,
		"mark refund failed", handlerID, handledAt, id)
}

// MarkRejectedTx 在事务内驳回待处理工单（订单保持已支付）。
func (s *RefundTicketStore) MarkRejectedTx(ctx context.Context, tx *sql.Tx, id, handlerID int64, rejectReason, handledAt string) error {
	return s.transitionTx(ctx, tx, `
		UPDATE refund_tickets SET status = 'rejected', handler_id = ?, reject_reason = ?, handled_at = ?
		WHERE id = ? AND status = 'pending'`,
		"mark rejected", handlerID, rejectReason, handledAt, id)
}

// MarkRefundedTx 在事务内把已受理工单置为已退款（退款回调成功）。
// 允许 refund_failed：退款接口超时/网络失败但微信实际退款成功时，以回调为准。
func (s *RefundTicketStore) MarkRefundedTx(ctx context.Context, tx *sql.Tx, id int64, handledAt string) error {
	return s.transitionTx(ctx, tx, `
		UPDATE refund_tickets SET status = 'refunded', handled_at = ?
		WHERE id = ? AND status IN ('approved', 'processing', 'refund_failed')`,
		"mark refunded", handledAt, id)
}

// transitionTx 执行条件状态 UPDATE，未命中返回 ErrConflict。
func (s *RefundTicketStore) transitionTx(ctx context.Context, tx *sql.Tx, query, action string, args ...any) error {
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of %s: %w", action, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

func scanRefundTicket(row rowScanner) (*model.RefundTicket, error) {
	var t model.RefundTicket
	if err := row.Scan(&t.ID, &t.TicketNo, &t.OrderID, &t.Reason, &t.Applicant, &t.Status,
		&t.HandlerID, &t.RejectReason, &t.HandledAt, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}
