package store

import (
	"context"
	"fmt"
	"strings"
)

// AdminOrderFilter 是后台订单列表的筛选与分页条件（API 文档 11.1）。
type AdminOrderFilter struct {
	EventID         int64
	Level           string
	DanceType       string
	Status          string
	IncludeCanceled bool
	Source          string
	PaidFrom        string
	PaidTo          string
	Keyword         string
	Page            int
	PageSize        int
}

// AdminOrderRow 是后台订单列表的一行（订单 + 报名/赛事/组别/选手摘要 + 支付流水号）。
type AdminOrderRow struct {
	OrderNo       string
	TransactionID *string
	EventID       int64
	EventName     string
	Level         string
	DanceType     string
	PlayerNo      string
	PlayerName    string
	IDType        string
	Amount        int64
	Status        string
	Source        string
	PaidAt        *string
	CreatedAt     string
}

// ListAdmin 分页查询后台订单列表。
// 待支付为支付流程中间态，始终不展示；已取消默认不展示，IncludeCanceled=true 时
// 作为对账留档返回。Status 显式指定时按 Status 精确筛选（可查已取消）。
func (s *OrderStore) ListAdmin(ctx context.Context, f AdminOrderFilter) ([]AdminOrderRow, int64, error) {
	where := []string{"o.status <> 'pending_payment'"}
	args := make([]any, 0, 8)
	if f.Status != "" {
		where = append(where, "o.status = ?")
		args = append(args, f.Status)
	} else if !f.IncludeCanceled {
		where = append(where, "o.status <> 'canceled'")
	}
	if f.EventID > 0 {
		where = append(where, "r.event_id = ?")
		args = append(args, f.EventID)
	}
	if f.Level != "" {
		where = append(where, "g.level = ?")
		args = append(args, f.Level)
	}
	if f.DanceType != "" {
		where = append(where, "r.dance_type = ?")
		args = append(args, f.DanceType)
	}
	if f.Source != "" {
		where = append(where, "o.source = ?")
		args = append(args, f.Source)
	}
	if f.PaidFrom != "" {
		where = append(where, "o.paid_at >= ?")
		args = append(args, f.PaidFrom)
	}
	if f.PaidTo != "" {
		where = append(where, "o.paid_at <= ?")
		args = append(args, f.PaidTo)
	}
	if f.Keyword != "" {
		like := "%" + f.Keyword + "%"
		where = append(where, `(pl.name LIKE ? OR o.order_no LIKE ?
			OR EXISTS (SELECT 1 FROM payments p
			           WHERE p.order_id = o.id AND p.type = 'pay' AND p.transaction_id LIKE ?))`)
		args = append(args, like, like, like)
	}
	cond := strings.Join(where, " AND ")
	from := `FROM orders o
		JOIN registrations r ON r.id = o.registration_id
		JOIN events e ON e.id = r.event_id
		JOIN event_groups g ON g.id = r.group_id
		JOIN players pl ON pl.id = r.player_id`

	var total int64
	if err := s.db.sql.QueryRowContext(ctx,
		`SELECT COUNT(1) `+from+` WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count admin orders: %w", err)
	}
	if total == 0 {
		return []AdminOrderRow{}, 0, nil
	}

	rows, err := s.db.sql.QueryContext(ctx, `
		SELECT o.order_no,
		       (SELECT p.transaction_id FROM payments p
		        WHERE p.order_id = o.id AND p.type = 'pay' AND p.status = 'success'
		        ORDER BY p.id LIMIT 1),
		       r.event_id, e.name, g.level, r.dance_type,
		       pl.player_no, pl.name, pl.id_type,
		       o.amount, o.status, o.source, o.paid_at, o.created_at `+from+`
		WHERE `+cond+`
		ORDER BY o.created_at DESC, o.id DESC
		LIMIT ? OFFSET ?`, append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin orders: %w", err)
	}
	defer rows.Close()

	list := make([]AdminOrderRow, 0, f.PageSize)
	for rows.Next() {
		var r AdminOrderRow
		if err := rows.Scan(&r.OrderNo, &r.TransactionID, &r.EventID, &r.EventName,
			&r.Level, &r.DanceType, &r.PlayerNo, &r.PlayerName, &r.IDType,
			&r.Amount, &r.Status, &r.Source, &r.PaidAt, &r.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan admin order: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate admin orders: %w", err)
	}
	return list, total, nil
}
