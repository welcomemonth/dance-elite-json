package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// RegistrationStore 是报名单（registrations 表）的数据访问层。
type RegistrationStore struct{ db *DB }

// NewRegistrationStore 构造报名单 store。
func NewRegistrationStore(db *DB) *RegistrationStore { return &RegistrationStore{db: db} }

// RegistrationParams 是创建报名单的字段（T9 提交报名）。
type RegistrationParams struct {
	EventID     int64
	GroupID     int64
	PlayerID    int64
	UserID      int64
	DanceType   string
	Institution *string
	Status      string // model.RegistrationStatusXxx
}

// CreateTx 在事务内创建报名单，返回新 id。
// 撞部分唯一索引 uq_registrations_in_flight（同人同报名单元已有进行中报名）
// 时返回 SQLite 唯一冲突错误，由 service 层映射为 41004。
func (s *RegistrationStore) CreateTx(ctx context.Context, tx *sql.Tx, p RegistrationParams) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO registrations (event_id, group_id, player_id, user_id, dance_type, institution, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.EventID, p.GroupID, p.PlayerID, p.UserID, p.DanceType, p.Institution, p.Status)
	if err != nil {
		return 0, fmt.Errorf("insert registration: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id of registration: %w", err)
	}
	return id, nil
}

// InFlightRegistration 是选手在某赛事的一条进行中报名（pending_payment/active）
// 及其所属报名单元信息，供 T9 判重（一人一组别 41003 / 同舞种重复 41004）。
type InFlightRegistration struct {
	RegistrationID int64
	GroupID        int64
	Level          string // 所属 event_groups.level
	DanceType      string
	Status         string
}

// InFlightByPlayerEventTx 在事务内查询选手在某赛事的全部进行中报名
// （含所属组别 level/dance_type，按组别名排序输出稳定）。
func (s *RegistrationStore) InFlightByPlayerEventTx(ctx context.Context, tx *sql.Tx, playerID, eventID int64) ([]InFlightRegistration, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT r.id, r.group_id, g.level, r.dance_type, r.status
		FROM registrations r
		JOIN event_groups g ON g.id = r.group_id
		WHERE r.player_id = ? AND r.event_id = ?
		  AND r.status IN ('pending_payment', 'active')
		ORDER BY g.level, r.dance_type`, playerID, eventID)
	if err != nil {
		return nil, fmt.Errorf("query in-flight registrations of player %d in event %d: %w", playerID, eventID, err)
	}
	defer rows.Close()

	list := make([]InFlightRegistration, 0, 2)
	for rows.Next() {
		var r InFlightRegistration
		if err := rows.Scan(&r.RegistrationID, &r.GroupID, &r.Level, &r.DanceType, &r.Status); err != nil {
			return nil, fmt.Errorf("scan in-flight registration: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate in-flight registrations of player %d in event %d: %w", playerID, eventID, err)
	}
	return list, nil
}

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
