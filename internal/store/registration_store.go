package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
)

// RegistrationStore 是报名单（registrations 表）的数据访问层。
type RegistrationStore struct{ db *DB }

// NewRegistrationStore 构造报名单 store。
func NewRegistrationStore(db *DB) *RegistrationStore { return &RegistrationStore{db: db} }

const registrationColumns = `id, event_id, group_id, player_id, user_id, dance_type,
	institution, status, source, cs_ticket_id, photo_path, audio_path,
	program_name, teacher_name, contact_phone, express_address,
	work_intro, special_req, material_updated_at, created_at, updated_at`

// Get 按 id 查询报名单。不存在返回 ErrNotFound。
func (s *RegistrationStore) Get(ctx context.Context, id int64) (*model.Registration, error) {
	return s.get(ctx, s.db.sql, id)
}

// GetTx 在事务内按 id 查询报名单（T12 取消事务内复核状态）。
// 不存在返回 ErrNotFound。
func (s *RegistrationStore) GetTx(ctx context.Context, tx *sql.Tx, id int64) (*model.Registration, error) {
	return s.get(ctx, tx, id)
}

func (s *RegistrationStore) get(ctx context.Context, q queryer, id int64) (*model.Registration, error) {
	registration, err := scanRegistration(q.QueryRowContext(ctx,
		`SELECT `+registrationColumns+` FROM registrations WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query registration %d: %w", id, err)
	}
	return registration, nil
}

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

// ActivateTx 在事务内把报名单从 pending_payment 置为 active（T11 支付成功）。
// 条件未命中（已 active / 已取消 / 已退赛）返回 ErrConflict，由调用方决定
// 整笔支付事务是否回滚（状态不一致必须回滚并告警）。
func (s *RegistrationStore) ActivateTx(ctx context.Context, tx *sql.Tx, id int64) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE registrations SET status = 'active' WHERE id = ? AND status = 'pending_payment'`, id)
	if err != nil {
		return fmt.Errorf("activate registration %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of registration %d activate: %w", id, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// CancelPendingTx 在事务内把报名单从 pending_payment 置为 canceled（T12 订单取消）。
// 条件未命中（已 active / 已取消 / 已退赛）返回 ErrConflict，取消事务须整体回滚
// （订单与报名状态不一致属数据异常）。
func (s *RegistrationStore) CancelPendingTx(ctx context.Context, tx *sql.Tx, id int64) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE registrations SET status = 'canceled' WHERE id = ? AND status = 'pending_payment'`, id)
	if err != nil {
		return fmt.Errorf("cancel registration %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of registration %d cancel: %w", id, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// MaterialUpdateParams 是更新参赛资料（API 文档 4.1）的字段补丁。
//
// nil 表示不修改该列；非 nil 的文本字段为空串时写入 NULL（清空）。
// PhotoPath/AudioPath 只在新文件上传时非 nil（文件不支持"清空"操作）。
// MaterialUpdatedAt 在任意一项更新时刷新。
type MaterialUpdateParams struct {
	PhotoPath         *string
	AudioPath         *string
	ProgramName       *string
	TeacherName       *string
	ContactPhone      *string
	ExpressAddress    *string
	WorkIntro         *string
	SpecialReq        *string
	MaterialUpdatedAt string
}

// UpdateMaterialsTx 在事务内更新报名单资料字段。条件限定本人且报名为
// active（T13：仅有效报名可修改资料）；条件未命中返回 ErrConflict
// （并发下状态已变化），不存在返回 ErrNotFound。
func (s *RegistrationStore) UpdateMaterialsTx(ctx context.Context, tx *sql.Tx, id, userID int64, p MaterialUpdateParams) error {
	sets := make([]string, 0, 9)
	args := make([]any, 0, 10)
	add := func(column string, value *string) {
		if value == nil {
			return
		}
		sets = append(sets, column+" = NULLIF(?, '')")
		args = append(args, *value)
	}
	add("photo_path", p.PhotoPath)
	add("audio_path", p.AudioPath)
	add("program_name", p.ProgramName)
	add("teacher_name", p.TeacherName)
	add("contact_phone", p.ContactPhone)
	add("express_address", p.ExpressAddress)
	add("work_intro", p.WorkIntro)
	add("special_req", p.SpecialReq)
	sets = append(sets, "material_updated_at = ?")
	args = append(args, p.MaterialUpdatedAt, id, userID)

	res, err := tx.ExecContext(ctx,
		`UPDATE registrations SET `+strings.Join(sets, ", ")+`
		 WHERE id = ? AND user_id = ? AND status = 'active'`, args...)
	if err != nil {
		return fmt.Errorf("update materials of registration %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of registration %d materials update: %w", id, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
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

// scanRegistration 把行扫描为报名单（指针列均为可空字段）。
func scanRegistration(row rowScanner) (*model.Registration, error) {
	var r model.Registration
	if err := row.Scan(&r.ID, &r.EventID, &r.GroupID, &r.PlayerID, &r.UserID, &r.DanceType,
		&r.Institution, &r.Status, &r.Source, &r.CSTicketID, &r.PhotoPath, &r.AudioPath,
		&r.ProgramName, &r.TeacherName, &r.ContactPhone, &r.ExpressAddress,
		&r.WorkIntro, &r.SpecialReq, &r.MaterialUpdatedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	return &r, nil
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
