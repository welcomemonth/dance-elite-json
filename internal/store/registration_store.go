package store

import (
	"context"
	"fmt"
	"strings"
)

// RegistrationStore 是报名单（registrations 表）的数据访问层。
// T7 仅提供小程序赛事接口所需的最小读取能力；完整的报名下单/取消
// 数据访问在 T9/T12 落地。
type RegistrationStore struct{ db *DB }

// NewRegistrationStore 构造报名单 store。
func NewRegistrationStore(db *DB) *RegistrationStore { return &RegistrationStore{db: db} }

// ActiveEventIDs 返回指定用户在给定赛事集合中存在有效报名（status='active'）
// 的赛事 ID 集，供赛事列表"已报名"角标批量查询。
func (s *RegistrationStore) ActiveEventIDs(ctx context.Context, userID int64, eventIDs []int64) (map[int64]bool, error) {
	out := make(map[int64]bool, len(eventIDs))
	if len(eventIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(eventIDs))
	args := make([]any, 0, len(eventIDs)+1)
	args = append(args, userID)
	for i, id := range eventIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	rows, err := s.db.sql.QueryContext(ctx,
		`SELECT DISTINCT event_id FROM registrations
			WHERE user_id = ? AND status = 'active' AND event_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("query active registrations of user %d: %w", userID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan active registration event id: %w", err)
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active registrations of user %d: %w", userID, err)
	}
	return out, nil
}
