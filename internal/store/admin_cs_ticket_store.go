package store

import (
	"context"
	"fmt"

	"github.com/meetmonth/dance-elite/internal/model"
)

// ListByOrderOrRegistration 返回与订单或报名单关联的客服工单（订单详情页使用）。
// 关联关系可能落在 related_order_id（登记时指定订单）或 related_registration_id
// （人工报名回填报名单），两者任一命中即返回，按写入顺序。
func (s *CSTicketStore) ListByOrderOrRegistration(ctx context.Context, orderID, registrationID int64) ([]model.CSTicket, error) {
	rows, err := s.db.sql.QueryContext(ctx, `
		SELECT `+csTicketColumns+` FROM cs_tickets
		WHERE related_order_id = ? OR related_registration_id = ?
		ORDER BY id`, orderID, registrationID)
	if err != nil {
		return nil, fmt.Errorf("query cs tickets of order %d/registration %d: %w", orderID, registrationID, err)
	}
	defer rows.Close()

	list := make([]model.CSTicket, 0, 1)
	for rows.Next() {
		t, err := scanCSTicket(rows)
		if err != nil {
			return nil, fmt.Errorf("scan cs ticket of order %d: %w", orderID, err)
		}
		list = append(list, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cs tickets of order %d: %w", orderID, err)
	}
	return list, nil
}
