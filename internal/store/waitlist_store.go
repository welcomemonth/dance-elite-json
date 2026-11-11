package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
)

// WaitlistStore 是候补队列（waitlists 表）的数据访问层（开发计划 T14）。
type WaitlistStore struct{ db *DB }

// NewWaitlistStore 构造候补队列 store。
func NewWaitlistStore(db *DB) *WaitlistStore { return &WaitlistStore{db: db} }

const waitlistColumns = `id, event_id, group_id, user_id, dance_type, name, id_card_hash,
	id_card_masked, institution, phone, status, position_no, notified_at, handled_at, created_at, updated_at`

// WaitlistParams 是创建候补记录的字段（T14 提交候补）。
type WaitlistParams struct {
	EventID      int64
	GroupID      int64
	UserID       int64
	DanceType    string
	Name         string
	IDCardHash   *string
	IDCardMasked *string
	Institution  *string
	Phone        string
	PositionNo   int
}

// Get 按 id 查询候补记录。不存在返回 ErrNotFound。
func (s *WaitlistStore) Get(ctx context.Context, id int64) (*model.Waitlist, error) {
	return s.get(ctx, s.db.sql, id)
}

// GetTx 在事务内按 id 查询候补记录（写操作前复核状态）。不存在返回 ErrNotFound。
func (s *WaitlistStore) GetTx(ctx context.Context, tx *sql.Tx, id int64) (*model.Waitlist, error) {
	return s.get(ctx, tx, id)
}

func (s *WaitlistStore) get(ctx context.Context, q queryer, id int64) (*model.Waitlist, error) {
	w, err := scanWaitlist(q.QueryRowContext(ctx,
		`SELECT `+waitlistColumns+` FROM waitlists WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query waitlist %d: %w", id, err)
	}
	return w, nil
}

// InsertTx 在事务内创建候补记录，返回新 id。撞 uq_waitlists_waiting
// （同报名单元同证件号已有排队中记录）时返回唯一冲突错误，由 service 映射 41010。
func (s *WaitlistStore) InsertTx(ctx context.Context, tx *sql.Tx, p WaitlistParams) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO waitlists (event_id, group_id, user_id, dance_type, name, id_card_hash,
			id_card_masked, institution, phone, status, position_no)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'waiting', ?)`,
		p.EventID, p.GroupID, p.UserID, p.DanceType, p.Name, p.IDCardHash,
		p.IDCardMasked, p.Institution, p.Phone, p.PositionNo)
	if err != nil {
		return 0, fmt.Errorf("insert waitlist of event %d group %d: %w", p.EventID, p.GroupID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id of waitlist: %w", err)
	}
	return id, nil
}

// WaitingExistsTx 在事务内查询同一报名单元是否已有排队中（waiting）的同证件号记录。
// hash 为空（无证件号登记）时返回 false——此类记录只能人工识别。
func (s *WaitlistStore) WaitingExistsTx(ctx context.Context, tx *sql.Tx, eventID, groupID int64, hash string) (bool, error) {
	if hash == "" {
		return false, nil
	}
	var n int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM waitlists
		WHERE event_id = ? AND group_id = ? AND id_card_hash = ? AND status = 'waiting'`,
		eventID, groupID, hash).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check waiting waitlist of event %d group %d: %w", eventID, groupID, err)
	}
	return n > 0, nil
}

// NextPositionTx 在事务内计算某报名单元的下一个队列序号（MAX + 1）。
// 事务以 BEGIN IMMEDIATE 串行化写操作，不会产生重号。
func (s *WaitlistStore) NextPositionTx(ctx context.Context, tx *sql.Tx, eventID, groupID int64) (int, error) {
	var next int
	err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(position_no), 0) + 1 FROM waitlists
		WHERE event_id = ? AND group_id = ?`, eventID, groupID).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("next position of event %d group %d: %w", eventID, groupID, err)
	}
	return next, nil
}

// CancelOwnTx 在事务内把用户本人的排队中候补置为 given_up（自助取消）。
// 条件未命中（非本人 / 状态非 waiting）返回 ErrConflict，由调用方回读区分。
func (s *WaitlistStore) CancelOwnTx(ctx context.Context, tx *sql.Tx, id, userID int64, handledAt string) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE waitlists SET status = 'given_up', handled_at = ?
		WHERE id = ? AND user_id = ? AND status = 'waiting'`, handledAt, id, userID)
	if err != nil {
		return fmt.Errorf("cancel waitlist %d of user %d: %w", id, userID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of waitlist %d cancel: %w", id, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// MyWaitlistRow 是「我的候补」列表的一行（候补 + 赛事/组别摘要）。
type MyWaitlistRow struct {
	WaitlistID int64
	EventID    int64
	EventName  string
	Level      string
	DanceType  string
	PositionNo int
	Status     string
	CreatedAt  string
}

// ListByUser 分页查询用户提交的候补记录（含已放弃/已移除等留档记录），
// 按 id 倒序。空结果返回空切片而非 nil。
func (s *WaitlistStore) ListByUser(ctx context.Context, userID int64, page, pageSize int) ([]MyWaitlistRow, int, error) {
	var total int
	if err := s.db.sql.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM waitlists WHERE user_id = ?`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count waitlists of user %d: %w", userID, err)
	}
	if total == 0 {
		return []MyWaitlistRow{}, 0, nil
	}

	rows, err := s.db.sql.QueryContext(ctx, `
		SELECT w.id, w.event_id, e.name, g.level, w.dance_type, w.position_no, w.status, w.created_at
		FROM waitlists w
		JOIN events e ON e.id = w.event_id
		JOIN event_groups g ON g.id = w.group_id
		WHERE w.user_id = ?
		ORDER BY w.id DESC
		LIMIT ? OFFSET ?`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("query waitlists of user %d: %w", userID, err)
	}
	defer rows.Close()

	list := make([]MyWaitlistRow, 0, pageSize)
	for rows.Next() {
		var r MyWaitlistRow
		if err := rows.Scan(&r.WaitlistID, &r.EventID, &r.EventName, &r.Level,
			&r.DanceType, &r.PositionNo, &r.Status, &r.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan waitlist row of user %d: %w", userID, err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate waitlists of user %d: %w", userID, err)
	}
	return list, total, nil
}

// AdminWaitlistFilter 是后台候补列表的筛选与分页条件。
type AdminWaitlistFilter struct {
	EventID   int64
	Level     string
	DanceType string
	Status    string
	Page      int
	PageSize  int
}

// AdminWaitlistRow 是后台候补列表的一行（候补 + 赛事/组别摘要）。
type AdminWaitlistRow struct {
	WaitlistID   int64
	PositionNo   int
	Name         string
	IDCardMasked *string
	Institution  *string
	Phone        string
	Status       string
	EventID      int64
	EventName    string
	GroupID      int64
	Level        string
	DanceType    string
	CreatedAt    string
	NotifiedAt   *string
	HandledAt    *string
}

// ListAdmin 分页查询后台候补队列：按赛事、组别分组，组内按队列序号排序
// （通知顺序依据）。空结果返回空切片而非 nil。
func (s *WaitlistStore) ListAdmin(ctx context.Context, f AdminWaitlistFilter) ([]AdminWaitlistRow, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	if f.EventID > 0 {
		where = append(where, "w.event_id = ?")
		args = append(args, f.EventID)
	}
	if f.Level != "" {
		where = append(where, "g.level = ?")
		args = append(args, f.Level)
	}
	if f.DanceType != "" {
		where = append(where, "w.dance_type = ?")
		args = append(args, f.DanceType)
	}
	if f.Status != "" {
		where = append(where, "w.status = ?")
		args = append(args, f.Status)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := s.db.sql.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM waitlists w
		JOIN event_groups g ON g.id = w.group_id
		WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count admin waitlists: %w", err)
	}
	if total == 0 {
		return []AdminWaitlistRow{}, 0, nil
	}

	page, pageSize := f.Page, f.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	rows, err := s.db.sql.QueryContext(ctx, `
		SELECT w.id, w.position_no, w.name, w.id_card_masked, w.institution, w.phone,
		       w.status, w.event_id, e.name, w.group_id, g.level, w.dance_type,
		       w.created_at, w.notified_at, w.handled_at
		FROM waitlists w
		JOIN events e ON e.id = w.event_id
		JOIN event_groups g ON g.id = w.group_id
		WHERE `+cond+`
		ORDER BY w.event_id, w.group_id, w.position_no, w.id
		LIMIT ? OFFSET ?`, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin waitlists: %w", err)
	}
	defer rows.Close()

	list := make([]AdminWaitlistRow, 0, pageSize)
	for rows.Next() {
		var r AdminWaitlistRow
		if err := rows.Scan(&r.WaitlistID, &r.PositionNo, &r.Name, &r.IDCardMasked,
			&r.Institution, &r.Phone, &r.Status, &r.EventID, &r.EventName, &r.GroupID,
			&r.Level, &r.DanceType, &r.CreatedAt, &r.NotifiedAt, &r.HandledAt); err != nil {
			return nil, 0, fmt.Errorf("scan admin waitlist: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate admin waitlists: %w", err)
	}
	return list, total, nil
}

// ReleasedQuota 返回筛选范围内报名单元的可用名额合计（quota - taken，负值按 0）。
// 名额释放联动提示用：管理员退赛释放名额后，候补队列列表据此提示
// "该组别已释放 N 个名额"（只提示，人工电话联系，不做自动通知）。
// eventID 为 0 时返回 0（未按赛事筛选时无提示意义）。
func (s *WaitlistStore) ReleasedQuota(ctx context.Context, eventID int64, level, danceType string) (int, error) {
	if eventID <= 0 {
		return 0, nil
	}
	var n int
	err := s.db.sql.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE WHEN quota > taken THEN quota - taken ELSE 0 END), 0)
		FROM event_groups
		WHERE event_id = ? AND (? = '' OR level = ?) AND (? = '' OR dance_type = ?)`,
		eventID, level, level, danceType, danceType).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("released quota of event %d: %w", eventID, err)
	}
	return n, nil
}

// NotifyTx 在事务内标记已通知（waiting/notified → notified），刷新 notified_at。
// 条件未命中返回 ErrConflict，由调用方回读区分 40401/41013。
func (s *WaitlistStore) NotifyTx(ctx context.Context, tx *sql.Tx, id int64, notifiedAt string) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE waitlists SET status = 'notified', notified_at = ?
		WHERE id = ? AND status IN ('waiting', 'notified')`, notifiedAt, id)
	if err != nil {
		return fmt.Errorf("notify waitlist %d: %w", id, err)
	}
	return affectedOrConflict(res, "waitlist", id, "notify")
}

// FinishTx 在事务内把候补记录置为终态（converted / removed）并写 handled_at。
// 仅 waiting/notified 可流转；条件未命中返回 ErrConflict。
func (s *WaitlistStore) FinishTx(ctx context.Context, tx *sql.Tx, id int64, status, handledAt string) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE waitlists SET status = ?, handled_at = ?
		WHERE id = ? AND status IN ('waiting', 'notified')`, status, handledAt, id)
	if err != nil {
		return fmt.Errorf("finish waitlist %d as %s: %w", id, status, err)
	}
	return affectedOrConflict(res, "waitlist", id, status)
}

// affectedOrConflict 把 RowsAffected = 0 统一包装为 ErrConflict（状态条件未命中）。
func affectedOrConflict(res sql.Result, kind string, id int64, action string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of %s %d %s: %w", kind, id, action, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

func scanWaitlist(row rowScanner) (*model.Waitlist, error) {
	var w model.Waitlist
	if err := row.Scan(&w.ID, &w.EventID, &w.GroupID, &w.UserID, &w.DanceType, &w.Name,
		&w.IDCardHash, &w.IDCardMasked, &w.Institution, &w.Phone, &w.Status, &w.PositionNo,
		&w.NotifiedAt, &w.HandledAt, &w.CreatedAt, &w.UpdatedAt); err != nil {
		return nil, err
	}
	return &w, nil
}
