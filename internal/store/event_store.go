package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
)

// EventStore 是赛事（events 表）的数据访问层。
type EventStore struct{ db *DB }

// NewEventStore 构造赛事 store。
func NewEventStore(db *DB) *EventStore { return &EventStore{db: db} }

const eventColumns = `id, season_id, name, level, city, venue, event_date, fee, cover_path,
	reg_start_at, reg_end_at, lottery_at, material_check_at, start_at, status,
	score_rules, prize_rules, refund_policy, work_duration_limit, process_desc, honor_desc,
	exclude_from_prize, audio_review_token, created_at, updated_at`

// EventParams 是创建赛事的完整字段集（service 层完成校验与模板预填）。
type EventParams struct {
	SeasonID          int64
	Name              string
	Level             string
	City              *string
	Venue             *string
	EventDate         *string
	Fee               int64
	RegStartAt        *string
	RegEndAt          *string
	LotteryAt         *string
	MaterialCheckAt   *string
	StartAt           *string
	Status            string
	ScoreRules        *string
	PrizeRules        *string
	RefundPolicy      *string
	WorkDurationLimit *string
	ProcessDesc       *string
	HonorDesc         *string
	ExcludeFromPrize  bool
}

// EventPatch 是编辑赛事的字段；nil 指针表示不修改该列。
// 时间线与级别是否可改由 service 层按赛事状态决定（可改时才放入 patch）。
type EventPatch struct {
	Name              *string
	Level             *string
	City              *string
	Venue             *string
	EventDate         *string
	Fee               *int64
	RegStartAt        *string
	RegEndAt          *string
	LotteryAt         *string
	MaterialCheckAt   *string
	StartAt           *string
	ScoreRules        *string
	PrizeRules        *string
	RefundPolicy      *string
	WorkDurationLimit *string
	ProcessDesc       *string
	HonorDesc         *string
	ExcludeFromPrize  *bool
}

// EventFilter 是后台赛事列表的筛选与分页条件。
type EventFilter struct {
	Status   string
	Level    string
	Keyword  string
	Page     int // 从 1 开始
	PageSize int
}

// Get 按 id 查询赛事。不存在返回 ErrNotFound。
func (s *EventStore) Get(ctx context.Context, id int64) (*model.Event, error) {
	event, err := scanEvent(s.db.sql.QueryRowContext(ctx,
		`SELECT `+eventColumns+` FROM events WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query event %d: %w", id, err)
	}
	return event, nil
}

// ListPublished 返回全部已发布赛事（排除 draft/unpublished/offline），按 id 倒序。
// 小程序端列表用：展示状态（upcoming/registering/reg_closed）由 service 层
// 按时间线推导，故此处不做状态筛选与分页（赛事数量小，内存推导即可）。
func (s *EventStore) ListPublished(ctx context.Context) ([]model.Event, error) {
	rows, err := s.db.sql.QueryContext(ctx,
		`SELECT `+eventColumns+` FROM events
			WHERE status IN ('registering', 'reg_closed', 'ongoing', 'finished')
			ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query published events: %w", err)
	}
	defer rows.Close()

	list := make([]model.Event, 0, 16)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan published event: %w", err)
		}
		list = append(list, *event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate published events: %w", err)
	}
	return list, nil
}

// List 返回筛选后的赛事分页（按 id 倒序）与总数。
func (s *EventStore) List(ctx context.Context, f EventFilter) ([]model.Event, int64, error) {
	where, args := []string{"1=1"}, []any{}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.Level != "" {
		where = append(where, "level = ?")
		args = append(args, f.Level)
	}
	if f.Keyword != "" {
		where = append(where, "name LIKE ?")
		args = append(args, "%"+f.Keyword+"%")
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := s.db.sql.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM events WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	page, pageSize := f.Page, f.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	rows, err := s.db.sql.QueryContext(ctx,
		`SELECT `+eventColumns+` FROM events WHERE `+cond+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var list []model.Event
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan event: %w", err)
		}
		list = append(list, *event)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate events: %w", err)
	}
	return list, total, nil
}

// Insert 在事务内创建赛事，返回新 id。
func (s *EventStore) Insert(ctx context.Context, tx *sql.Tx, p EventParams) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO events (season_id, name, level, city, venue, event_date, fee,
			reg_start_at, reg_end_at, lottery_at, material_check_at, start_at, status,
			score_rules, prize_rules, refund_policy, work_duration_limit, process_desc, honor_desc,
			exclude_from_prize)
		VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?,
			NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?,
			NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
			?)`,
		p.SeasonID, p.Name, p.Level, optStr(p.City), optStr(p.Venue), optStr(p.EventDate), p.Fee,
		optStr(p.RegStartAt), optStr(p.RegEndAt), optStr(p.LotteryAt),
		optStr(p.MaterialCheckAt), optStr(p.StartAt), p.Status,
		optStr(p.ScoreRules), optStr(p.PrizeRules), optStr(p.RefundPolicy),
		optStr(p.WorkDurationLimit), optStr(p.ProcessDesc), optStr(p.HonorDesc),
		p.ExcludeFromPrize)
	if err != nil {
		return 0, fmt.Errorf("insert event %q: %w", p.Name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id of event %q: %w", p.Name, err)
	}
	return id, nil
}

// Update 在事务内按非 nil 字段更新赛事，合并为单条 UPDATE
// （updated_at 触发器只触发一次）。列名为代码内常量，不接收外部输入。
func (s *EventStore) Update(ctx context.Context, tx *sql.Tx, id int64, patch EventPatch) error {
	sets := make([]string, 0, 18)
	args := make([]any, 0, 18)
	add := func(column string, value any) {
		sets = append(sets, column+" = ?")
		args = append(args, value)
	}
	if patch.Name != nil {
		add("name", *patch.Name)
	}
	if patch.Level != nil {
		add("level", *patch.Level)
	}
	if patch.City != nil {
		add("city", nilIfEmpty(*patch.City))
	}
	if patch.Venue != nil {
		add("venue", nilIfEmpty(*patch.Venue))
	}
	if patch.EventDate != nil {
		add("event_date", nilIfEmpty(*patch.EventDate))
	}
	if patch.Fee != nil {
		add("fee", *patch.Fee)
	}
	if patch.RegStartAt != nil {
		add("reg_start_at", nilIfEmpty(*patch.RegStartAt))
	}
	if patch.RegEndAt != nil {
		add("reg_end_at", nilIfEmpty(*patch.RegEndAt))
	}
	if patch.LotteryAt != nil {
		add("lottery_at", nilIfEmpty(*patch.LotteryAt))
	}
	if patch.MaterialCheckAt != nil {
		add("material_check_at", nilIfEmpty(*patch.MaterialCheckAt))
	}
	if patch.StartAt != nil {
		add("start_at", nilIfEmpty(*patch.StartAt))
	}
	if patch.ScoreRules != nil {
		add("score_rules", nilIfEmpty(*patch.ScoreRules))
	}
	if patch.PrizeRules != nil {
		add("prize_rules", nilIfEmpty(*patch.PrizeRules))
	}
	if patch.RefundPolicy != nil {
		add("refund_policy", nilIfEmpty(*patch.RefundPolicy))
	}
	if patch.WorkDurationLimit != nil {
		add("work_duration_limit", nilIfEmpty(*patch.WorkDurationLimit))
	}
	if patch.ProcessDesc != nil {
		add("process_desc", nilIfEmpty(*patch.ProcessDesc))
	}
	if patch.HonorDesc != nil {
		add("honor_desc", nilIfEmpty(*patch.HonorDesc))
	}
	if patch.ExcludeFromPrize != nil {
		add("exclude_from_prize", *patch.ExcludeFromPrize)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	if _, err := tx.ExecContext(ctx,
		`UPDATE events SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return fmt.Errorf("update event %d: %w", id, err)
	}
	return nil
}

// TransitionStatus 在事务内做条件状态流转：仅当赛事当前状态在 from 集合内才更新。
// 当前状态不在集合内返回 ErrConflict（service 层映射为 41012/40901），不存在返回 ErrNotFound。
func (s *EventStore) TransitionStatus(ctx context.Context, tx *sql.Tx, id int64, from []string, to string) error {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(from)), ",")
	args := []any{to, id}
	for _, st := range from {
		args = append(args, st)
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE events SET status = ? WHERE id = ? AND status IN (`+placeholders+`)`, args...)
	if err != nil {
		return fmt.Errorf("transition event %d to %q: %w", id, to, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of event %d transition: %w", id, err)
	}
	if n == 0 {
		// 注意：连接池为单连接（MaxOpenConns=1），事务内不得再经 s.db.sql 查询，
		// 否则等待空闲连接会自死锁；存在性检查必须用同一事务连接完成。
		var cnt int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM events WHERE id = ?`, id).Scan(&cnt); err != nil {
			return fmt.Errorf("check existence of event %d: %w", id, err)
		}
		if cnt == 0 {
			return ErrNotFound
		}
		return ErrConflict
	}
	return nil
}

// eventColumns 与 setColumn 已随单条 UPDATE 重构移除。

func optStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func scanEvent(row rowScanner) (*model.Event, error) {
	var e model.Event
	if err := row.Scan(&e.ID, &e.SeasonID, &e.Name, &e.Level, &e.City, &e.Venue,
		&e.EventDate, &e.Fee, &e.CoverPath,
		&e.RegStartAt, &e.RegEndAt, &e.LotteryAt, &e.MaterialCheckAt, &e.StartAt,
		&e.Status, &e.ScoreRules, &e.PrizeRules, &e.RefundPolicy, &e.WorkDurationLimit,
		&e.ProcessDesc, &e.HonorDesc, &e.ExcludeFromPrize, &e.AudioReviewToken,
		&e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	return &e, nil
}

// EventGroupStore 是赛事组别×舞种配置（event_groups 表）的数据访问层。
type EventGroupStore struct{ db *DB }

// NewEventGroupStore 构造组别 store。
func NewEventGroupStore(db *DB) *EventGroupStore { return &EventGroupStore{db: db} }

const eventGroupColumns = `id, event_id, level, dance_type, birth_year_from, birth_year_to,
	quota, taken, enabled, created_at, updated_at`

// EventGroupParams 是创建组别的字段。
type EventGroupParams struct {
	EventID       int64
	Level         string
	DanceType     string
	BirthYearFrom int
	BirthYearTo   int
	Quota         int
}

// ListByEvent 返回某赛事的全部组别（按 level、dance_type 排序，输出稳定）。
func (s *EventGroupStore) ListByEvent(ctx context.Context, eventID int64) ([]model.EventGroup, error) {
	return s.queryGroups(ctx, `SELECT `+eventGroupColumns+
		` FROM event_groups WHERE event_id = ? ORDER BY level, dance_type`, eventID)
}

// ListByEvents 批量返回多个赛事的组别（后台列表用，避免 N+1；结果按 event_id 归组）。
func (s *EventGroupStore) ListByEvents(ctx context.Context, eventIDs []int64) (map[int64][]model.EventGroup, error) {
	out := make(map[int64][]model.EventGroup, len(eventIDs))
	if len(eventIDs) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(eventIDs)), ",")
	args := make([]any, len(eventIDs))
	for i, id := range eventIDs {
		args[i] = id
	}
	groups, err := s.queryGroups(ctx,
		`SELECT `+eventGroupColumns+` FROM event_groups WHERE event_id IN (`+placeholders+`) ORDER BY event_id, level, dance_type`, args...)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		out[g.EventID] = append(out[g.EventID], g)
	}
	return out, nil
}

// CountEnabledTx 在事务内统计某赛事启用状态的报名单元数（发布校验用）。
func (s *EventGroupStore) CountEnabledTx(ctx context.Context, tx *sql.Tx, eventID int64) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM event_groups WHERE event_id = ? AND enabled = 1`, eventID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count enabled groups of event %d: %w", eventID, err)
	}
	return n, nil
}

// queryGroups 执行组别查询并扫描为切片（空结果返回空切片而非 nil）。
func (s *EventGroupStore) queryGroups(ctx context.Context, query string, args ...any) ([]model.EventGroup, error) {
	rows, err := s.db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query event groups: %w", err)
	}
	defer rows.Close()

	list := make([]model.EventGroup, 0, 16)
	for rows.Next() {
		g, err := scanEventGroup(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event group: %w", err)
		}
		list = append(list, *g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event groups: %w", err)
	}
	return list, nil
}

// Get 按 id 查询组别。不存在返回 ErrNotFound。
func (s *EventGroupStore) Get(ctx context.Context, id int64) (*model.EventGroup, error) {
	g, err := scanEventGroup(s.db.sql.QueryRowContext(ctx,
		`SELECT `+eventGroupColumns+` FROM event_groups WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query event group %d: %w", id, err)
	}
	return g, nil
}

// Insert 在事务内创建一个报名单元。
func (s *EventGroupStore) Insert(ctx context.Context, tx *sql.Tx, p EventGroupParams) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO event_groups (event_id, level, dance_type, birth_year_from, birth_year_to, quota)
		VALUES (?, ?, ?, ?, ?, ?)`,
		p.EventID, p.Level, p.DanceType, p.BirthYearFrom, p.BirthYearTo, p.Quota)
	if err != nil {
		return 0, fmt.Errorf("insert group %s/%s of event %d: %w", p.Level, p.DanceType, p.EventID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id of group %s/%s: %w", p.Level, p.DanceType, err)
	}
	return id, nil
}

// UpdateQuota 在事务内调大名额。quota 必须大于当前值且不小于 taken（service 已校验，
// 此处 UPDATE 带条件兜底，防止并发/漏校验把名额调小）。
func (s *EventGroupStore) UpdateQuota(ctx context.Context, tx *sql.Tx, id int64, quota int) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE event_groups SET quota = ? WHERE id = ? AND quota < ? AND ? >= taken`,
		quota, id, quota, quota)
	if err != nil {
		return fmt.Errorf("update quota of group %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of group %d quota update: %w", id, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// SetQuota 在事务内直接设置名额（草稿/未发布赛事的全量编辑用，可增可减；
// 仍不允许小于已占名额 taken）。published 赛事的调整必须走 UpdateQuota（只能调大）。
func (s *EventGroupStore) SetQuota(ctx context.Context, tx *sql.Tx, id int64, quota int) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE event_groups SET quota = ? WHERE id = ? AND ? >= taken`,
		quota, id, quota)
	if err != nil {
		return fmt.Errorf("set quota of group %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of group %d quota set: %w", id, err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// SetEnabled 在事务内开启/关闭报名单元（关闭不影响已支付报名）。
func (s *EventGroupStore) SetEnabled(ctx context.Context, tx *sql.Tx, id int64, enabled bool) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE event_groups SET enabled = ? WHERE id = ?`, enabled, id)
	if err != nil {
		return fmt.Errorf("set enabled of group %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected of group %d enabled update: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanEventGroup(row rowScanner) (*model.EventGroup, error) {
	var g model.EventGroup
	if err := row.Scan(&g.ID, &g.EventID, &g.Level, &g.DanceType,
		&g.BirthYearFrom, &g.BirthYearTo, &g.Quota, &g.Taken, &g.Enabled,
		&g.CreatedAt, &g.UpdatedAt); err != nil {
		return nil, err
	}
	return &g, nil
}
