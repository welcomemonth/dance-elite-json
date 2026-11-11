package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/meetmonth/dance-elite/internal/model"
)

// OrderStore 是订单（orders 表）的数据访问层（T9 报名下单落地）。
type OrderStore struct{ db *DB }

// NewOrderStore 构造订单 store。
func NewOrderStore(db *DB) *OrderStore { return &OrderStore{db: db} }

// orderQualifiedColumns 是订单查询的限定列名（orders 表含 id/created_at/updated_at
// 等通用列，JOIN 时必须加 o. 前缀避免歧义）。
const orderQualifiedColumns = `o.id, o.order_no, o.registration_id, o.user_id, o.amount, o.status,
	o.request_id, o.prepay_id, o.paid_at, o.canceled_at, o.cancel_reason, o.created_at, o.updated_at`

// Get 按 id 查询订单。不存在返回 ErrNotFound。
func (s *OrderStore) Get(ctx context.Context, id int64) (*model.Order, error) {
	return s.get(ctx, s.db.sql, id)
}

// GetTx 在事务内按 id 查询订单（T12 取消事务内复核状态）。
// 不存在返回 ErrNotFound。
func (s *OrderStore) GetTx(ctx context.Context, tx *sql.Tx, id int64) (*model.Order, error) {
	return s.get(ctx, tx, id)
}

func (s *OrderStore) get(ctx context.Context, q queryer, id int64) (*model.Order, error) {
	order, err := scanOrder(q.QueryRowContext(ctx,
		`SELECT `+orderQualifiedColumns+` FROM orders o WHERE o.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query order %d: %w", id, err)
	}
	return order, nil
}

// GetByRequest 按 (user_id, request_id) 查询订单（非事务读，供测试与 T12 取消等
// 场景定位订单）。不存在时返回 (nil, nil)。
func (s *OrderStore) GetByRequest(ctx context.Context, userID int64, requestID string) (*model.Order, error) {
	order, err := scanOrder(s.db.sql.QueryRowContext(ctx,
		`SELECT `+orderQualifiedColumns+` FROM orders o WHERE o.user_id = ? AND o.request_id = ?`,
		userID, requestID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query order by request %q of user %d: %w", requestID, userID, err)
	}
	return order, nil
}

// GetByOrderNo 按商户订单号查询订单（T11 支付回调以 out_trade_no 定位）。
// 不存在返回 ErrNotFound。
func (s *OrderStore) GetByOrderNo(ctx context.Context, orderNo string) (*model.Order, error) {
	return s.getByOrderNo(ctx, s.db.sql, orderNo)
}

// GetByOrderNoTx 在事务内按商户订单号查询订单（T11 回调/查单结账）。
// 不存在返回 ErrNotFound。
func (s *OrderStore) GetByOrderNoTx(ctx context.Context, tx *sql.Tx, orderNo string) (*model.Order, error) {
	return s.getByOrderNo(ctx, tx, orderNo)
}

// queryer 是 *sql.DB 与 *sql.Tx 的公共查询接口（读路径保持一致）。
type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *OrderStore) getByOrderNo(ctx context.Context, q queryer, orderNo string) (*model.Order, error) {
	order, err := scanOrder(q.QueryRowContext(ctx,
		`SELECT `+orderQualifiedColumns+` FROM orders o WHERE o.order_no = ?`, orderNo))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query order by order_no %q: %w", orderNo, err)
	}
	return order, nil
}

// MarkPaidTx 在事务内条件把订单置为已支付（仅 pending_payment）。
// 条件未命中（订单已支付/已取消/已退款）返回 ErrConflict，调用方回读区分。
func (s *OrderStore) MarkPaidTx(ctx context.Context, tx *sql.Tx, id int64, paidAt string) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE orders SET status = 'paid', paid_at = ? WHERE id = ? AND status = 'pending_payment'`,
		paidAt, id)
	if err != nil {
		return fmt.Errorf("mark order %d paid: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of order %d paid update: %w", id, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// SetPrepayIDTx 在事务内条件写入 prepay_id：仅当订单仍为 pending_payment
// 且未写入过（并发预下单只有一个胜出，另一个复用胜出的 prepay_id）。
// 条件未命中返回 ErrConflict（service 层回读后复用或报 40901）。
func (s *OrderStore) SetPrepayIDTx(ctx context.Context, tx *sql.Tx, id int64, prepayID string) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE orders SET prepay_id = ? WHERE id = ? AND status = 'pending_payment' AND prepay_id IS NULL`,
		prepayID, id)
	if err != nil {
		return fmt.Errorf("set prepay_id of order %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of order %d prepay update: %w", id, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// CancelTx 在事务内条件把待支付订单置为已取消（T12）：
// 仅 status='pending_payment' 时写入取消原因与取消时间。
// 条件未命中（已支付/已取消/退款中）返回 ErrConflict，由调用方回读区分。
func (s *OrderStore) CancelTx(ctx context.Context, tx *sql.Tx, id int64, reason, canceledAt string) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE orders SET status = 'canceled', cancel_reason = ?, canceled_at = ?
		 WHERE id = ? AND status = 'pending_payment'`,
		reason, canceledAt, id)
	if err != nil {
		return fmt.Errorf("cancel order %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of order %d cancel: %w", id, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// OrderHistoryRow 是用户历史订单列表的一行（订单 + 报名/赛事/组别/选手摘要）。
type OrderHistoryRow struct {
	OrderID        int64
	OrderNo        string
	RegistrationID int64
	EventID        int64
	EventName      string
	Level          string
	DanceType      string
	PlayerName     string
	Amount         int64
	Status         string
	CancelReason   *string
	PaidAt         *string
	CanceledAt     *string
	CreatedAt      string
}

// ListHistoryByUser 分页查询用户历史订单（T12「我的-历史订单」）。
// 待支付订单属于支付流程中间态，前端不可见，不进入历史列表。
// 按 id 倒序（创建时间单调递增）返回，结果稳定。
func (s *OrderStore) ListHistoryByUser(ctx context.Context, userID int64, page, pageSize int) ([]OrderHistoryRow, int, error) {
	const visible = `o.user_id = ? AND o.status <> 'pending_payment'`

	var total int
	if err := s.db.sql.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM orders o WHERE `+visible, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count history orders of user %d: %w", userID, err)
	}
	if total == 0 {
		return []OrderHistoryRow{}, 0, nil
	}

	rows, err := s.db.sql.QueryContext(ctx, `
		SELECT o.id, o.order_no, o.registration_id, r.event_id, e.name, g.level,
		       r.dance_type, p.name, o.amount, o.status, o.cancel_reason,
		       o.paid_at, o.canceled_at, o.created_at
		FROM orders o
		JOIN registrations r ON r.id = o.registration_id
		JOIN events e ON e.id = r.event_id
		JOIN event_groups g ON g.id = r.group_id
		JOIN players p ON p.id = r.player_id
		WHERE `+visible+`
		ORDER BY o.id DESC
		LIMIT ? OFFSET ?`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("query history orders of user %d: %w", userID, err)
	}
	defer rows.Close()

	list := make([]OrderHistoryRow, 0, pageSize)
	for rows.Next() {
		var row OrderHistoryRow
		if err := rows.Scan(&row.OrderID, &row.OrderNo, &row.RegistrationID, &row.EventID,
			&row.EventName, &row.Level, &row.DanceType, &row.PlayerName, &row.Amount,
			&row.Status, &row.CancelReason, &row.PaidAt, &row.CanceledAt, &row.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan history order: %w", err)
		}
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate history orders of user %d: %w", userID, err)
	}
	return list, total, nil
}

func scanOrder(row rowScanner) (*model.Order, error) {
	var o model.Order
	if err := row.Scan(&o.ID, &o.OrderNo, &o.RegistrationID, &o.UserID, &o.Amount, &o.Status,
		&o.RequestID, &o.PrepayID, &o.PaidAt, &o.CanceledAt, &o.CancelReason,
		&o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	return &o, nil
}

// OrderParams 是创建订单的字段（T9 提交报名）。
type OrderParams struct {
	OrderNo        string
	RegistrationID int64
	UserID         int64
	Amount         int64
	Status         string // model.OrderStatusXxx
	RequestID      *string
}

// CreateTx 在事务内创建订单，返回新 id。
// 撞 uq_orders_request（同人同 request_id 重复下单）时返回唯一冲突错误，
// 由 service 层映射（正常路径已被幂等查重拦截，此处为兜底）。
func (s *OrderStore) CreateTx(ctx context.Context, tx *sql.Tx, p OrderParams) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO orders (order_no, registration_id, user_id, amount, status, request_id)
		VALUES (?, ?, ?, ?, ?, ?)`,
		p.OrderNo, p.RegistrationID, p.UserID, p.Amount, p.Status, p.RequestID)
	if err != nil {
		return 0, fmt.Errorf("insert order %q: %w", p.OrderNo, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id of order %q: %w", p.OrderNo, err)
	}
	return id, nil
}

// OrderWithContext 是订单及其关联报名单元/赛事信息，供幂等查重直接
// 构造提交报名响应（无需二次查询）。
type OrderWithContext struct {
	Order           model.Order
	Level           string // 报名单元 level（服务端判定回填）
	DanceType       string
	MaterialCheckAt *string // 资料上传截止（= 核对舞蹈音频时间）
}

// GetByRequestTx 在事务内按 (user_id, request_id) 查询订单及关联信息
// （T9 幂等查重）。不存在时返回 (nil, nil)。
func (s *OrderStore) GetByRequestTx(ctx context.Context, tx *sql.Tx, userID int64, requestID string) (*OrderWithContext, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT `+orderQualifiedColumns+`, g.level, r.dance_type, e.material_check_at
		FROM orders o
		JOIN registrations r ON r.id = o.registration_id
		JOIN event_groups g ON g.id = r.group_id
		JOIN events e ON e.id = r.event_id
		WHERE o.user_id = ? AND o.request_id = ?`, userID, requestID)
	out, err := scanOrderWithContext(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query order by request %q of user %d: %w", requestID, userID, err)
	}
	return out, nil
}

func scanOrderWithContext(row rowScanner) (*OrderWithContext, error) {
	var (
		out OrderWithContext
		o   model.Order
	)
	if err := row.Scan(&o.ID, &o.OrderNo, &o.RegistrationID, &o.UserID, &o.Amount, &o.Status,
		&o.RequestID, &o.PrepayID, &o.PaidAt, &o.CanceledAt, &o.CancelReason,
		&o.CreatedAt, &o.UpdatedAt,
		&out.Level, &out.DanceType, &out.MaterialCheckAt); err != nil {
		return nil, err
	}
	out.Order = o
	return &out, nil
}
