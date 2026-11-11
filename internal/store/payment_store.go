package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/meetmonth/dance-elite/internal/model"
)

// PaymentStore 是支付流水（payments 表）的数据访问层（T11 支付回调）。
type PaymentStore struct{ db *DB }

// NewPaymentStore 构造支付流水 store。
func NewPaymentStore(db *DB) *PaymentStore { return &PaymentStore{db: db} }

const paymentColumns = `id, order_id, type, prepay_id, transaction_id, refund_id,
	amount, status, raw_callback, notified_at, created_at, updated_at`

// GetByTransactionIDTx 在事务内按微信支付订单号查询流水（回调幂等键）。
// 不存在时返回 (nil, nil)。
func (s *PaymentStore) GetByTransactionIDTx(ctx context.Context, tx *sql.Tx, transactionID string) (*model.Payment, error) {
	payment, err := scanPayment(tx.QueryRowContext(ctx,
		`SELECT `+paymentColumns+` FROM payments WHERE transaction_id = ?`, transactionID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query payment by transaction %q: %w", transactionID, err)
	}
	return payment, nil
}

// GetByRefundIDTx 在事务内按微信退款单号查询流水（退款回调幂等键）。
// 不存在时返回 (nil, nil)。
func (s *PaymentStore) GetByRefundIDTx(ctx context.Context, tx *sql.Tx, refundID string) (*model.Payment, error) {
	payment, err := scanPayment(tx.QueryRowContext(ctx,
		`SELECT `+paymentColumns+` FROM payments WHERE refund_id = ?`, refundID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query payment by refund %q: %w", refundID, err)
	}
	return payment, nil
}

// CompleteRefundTx 在事务内补齐退款回调信息：金额以回调为准、置成功、
// 记录原始报文与回调时间（退款流水在同意退款时已创建）。
func (s *PaymentStore) CompleteRefundTx(ctx context.Context, tx *sql.Tx, id, amount int64, rawCallback, notifiedAt string) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE payments SET amount = ?, status = 'success', raw_callback = ?, notified_at = ?
		WHERE id = ? AND type = 'refund'`,
		amount, rawCallback, notifiedAt, id)
	if err != nil {
		return fmt.Errorf("complete refund payment %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of refund payment %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkRefundFailedTx 在事务内把退款流水置为失败（退款接口失败/关闭回调）。
func (s *PaymentStore) MarkRefundFailedTx(ctx context.Context, tx *sql.Tx, id int64) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE payments SET status = 'failed' WHERE id = ? AND type = 'refund'`, id)
	if err != nil {
		return fmt.Errorf("mark refund payment %d failed: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of refund payment %d failed: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListByOrder 返回订单的全部支付/退款流水（按写入顺序）。
func (s *PaymentStore) ListByOrder(ctx context.Context, orderID int64) ([]model.Payment, error) {
	rows, err := s.db.sql.QueryContext(ctx,
		`SELECT `+paymentColumns+` FROM payments WHERE order_id = ? ORDER BY id`, orderID)
	if err != nil {
		return nil, fmt.Errorf("query payments of order %d: %w", orderID, err)
	}
	defer rows.Close()

	list := make([]model.Payment, 0, 2)
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan payment of order %d: %w", orderID, err)
		}
		list = append(list, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate payments of order %d: %w", orderID, err)
	}
	return list, nil
}

// PaymentParams 是写入一条支付流水的字段（T11 支付回调）。
type PaymentParams struct {
	OrderID       int64
	Type          string // model.PaymentTypeXxx
	PrepayID      *string
	TransactionID *string
	RefundID      *string
	Amount        int64
	Status        string // model.PaymentStatusXxx
	RawCallback   *string
	NotifiedAt    *string
}

// CreateTx 在事务内写入支付流水，返回新 id。
// 撞 transaction_id 唯一约束时返回错误，由 service 层回查后按幂等处理
// （正常路径已先查重，此处仅在并发回调时兜底）。
func (s *PaymentStore) CreateTx(ctx context.Context, tx *sql.Tx, p PaymentParams) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO payments (order_id, type, prepay_id, transaction_id, refund_id,
			amount, status, raw_callback, notified_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.OrderID, p.Type, p.PrepayID, p.TransactionID, p.RefundID,
		p.Amount, p.Status, p.RawCallback, p.NotifiedAt)
	if err != nil {
		return 0, fmt.Errorf("insert payment of order %d: %w", p.OrderID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id of payment of order %d: %w", p.OrderID, err)
	}
	return id, nil
}

func scanPayment(row rowScanner) (*model.Payment, error) {
	var p model.Payment
	if err := row.Scan(&p.ID, &p.OrderID, &p.Type, &p.PrepayID, &p.TransactionID, &p.RefundID,
		&p.Amount, &p.Status, &p.RawCallback, &p.NotifiedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}
