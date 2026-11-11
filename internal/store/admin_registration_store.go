package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
)

// AdminRegistrationFilter 是后台报名名单的筛选与分页条件（开发计划 T17）。
type AdminRegistrationFilter struct {
	EventID       int64
	GroupID       int64
	Level         string
	DanceType     string
	Status        string
	MaterialState string // complete / incomplete / 空
	MissingItem   string // photo / audio / program_name / contact_phone
	Keyword       string // 选手姓名 / player_no / 订单号
	Page          int
	PageSize      int
}

// AdminRegistrationRow 是后台报名名单的一行（报名单 + 赛事/组别/选手/订单摘要）。
type AdminRegistrationRow struct {
	Registration model.Registration
	OrderNo      *string
	EventName    string
	Level        string
	PlayerNo     string
	PlayerName   string
	IDType       string
	ContactPhone *string // 报名单填写值，缺省回退用户手机号
}

// adminRegistrationSelectColumns 是 JOIN 查询的限定列（与 scanAdminRegistration 顺序一致）。
const adminRegistrationSelectColumns = `r.id, r.event_id, r.group_id, r.player_id, r.user_id,
	r.dance_type, r.institution, r.status, r.source, r.cs_ticket_id, r.photo_path,
	r.audio_path, r.program_name, r.teacher_name, r.contact_phone, r.express_address,
	r.work_intro, r.special_req, r.material_updated_at, r.created_at, r.updated_at`

// adminRegistrationFrom 是报名名单固定 JOIN（users 用于手机号回退，orders 1:1）。
const adminRegistrationFrom = `FROM registrations r
	JOIN events e ON e.id = r.event_id
	JOIN event_groups g ON g.id = r.group_id
	JOIN players pl ON pl.id = r.player_id
	JOIN users u ON u.id = r.user_id
	LEFT JOIN orders o ON o.registration_id = r.id`

// ListAdmin 分页查询后台报名名单。
func (s *RegistrationStore) ListAdmin(ctx context.Context, f AdminRegistrationFilter) ([]AdminRegistrationRow, int64, error) {
	where := []string{"1=1"}
	args := make([]any, 0, 8)
	if f.Status != "" {
		where = append(where, "r.status = ?")
		args = append(args, f.Status)
	} else {
		where = append(where, "r.status = 'active'")
	}
	if f.EventID > 0 {
		where = append(where, "r.event_id = ?")
		args = append(args, f.EventID)
	}
	if f.GroupID > 0 {
		where = append(where, "r.group_id = ?")
		args = append(args, f.GroupID)
	}
	if f.Level != "" {
		where = append(where, "g.level = ?")
		args = append(args, f.Level)
	}
	if f.DanceType != "" {
		where = append(where, "r.dance_type = ?")
		args = append(args, f.DanceType)
	}
	if f.MissingItem != "" {
		if column, ok := materialItemColumn(f.MissingItem); ok {
			where = append(where, "NOT ("+materialPresentSQL(column)+")")
		}
	}
	if f.MaterialState != "" {
		cond := materialCompleteSQL()
		if f.MaterialState == "incomplete" {
			cond = "NOT (" + cond + ")"
		}
		where = append(where, cond)
	}
	if f.Keyword != "" {
		like := "%" + f.Keyword + "%"
		where = append(where, "(pl.name LIKE ? OR pl.player_no LIKE ? OR o.order_no LIKE ?)")
		args = append(args, like, like, like)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := s.db.sql.QueryRowContext(ctx,
		`SELECT COUNT(1) `+adminRegistrationFrom+` WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count admin registrations: %w", err)
	}
	if total == 0 {
		return []AdminRegistrationRow{}, 0, nil
	}

	rows, err := s.db.sql.QueryContext(ctx, `
		SELECT `+adminRegistrationSelectColumns+`,
		       o.order_no, e.name, g.level, pl.player_no, pl.name, pl.id_type,
		       COALESCE(r.contact_phone, u.phone) `+adminRegistrationFrom+`
		WHERE `+cond+`
		ORDER BY r.created_at DESC, r.id DESC
		LIMIT ? OFFSET ?`, append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query admin registrations: %w", err)
	}
	defer rows.Close()

	list := make([]AdminRegistrationRow, 0, f.PageSize)
	for rows.Next() {
		row, err := scanAdminRegistration(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan admin registration: %w", err)
		}
		list = append(list, *row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate admin registrations: %w", err)
	}
	return list, total, nil
}

// scanAdminRegistration 扫描后台报名名单一行。
func scanAdminRegistration(row rowScanner) (*AdminRegistrationRow, error) {
	var r AdminRegistrationRow
	reg := &r.Registration
	if err := row.Scan(&reg.ID, &reg.EventID, &reg.GroupID, &reg.PlayerID, &reg.UserID,
		&reg.DanceType, &reg.Institution, &reg.Status, &reg.Source, &reg.CSTicketID,
		&reg.PhotoPath, &reg.AudioPath, &reg.ProgramName, &reg.TeacherName,
		&reg.ContactPhone, &reg.ExpressAddress, &reg.WorkIntro, &reg.SpecialReq,
		&reg.MaterialUpdatedAt, &reg.CreatedAt, &reg.UpdatedAt,
		&r.OrderNo, &r.EventName, &r.Level, &r.PlayerNo, &r.PlayerName, &r.IDType,
		&r.ContactPhone); err != nil {
		return nil, err
	}
	return &r, nil
}

// materialItemColumn 把缺失资料项名映射为报名单列名。
func materialItemColumn(field string) (string, bool) {
	switch field {
	case model.MaterialFieldPhoto:
		return "photo_path", true
	case model.MaterialFieldAudio:
		return "audio_path", true
	case model.MaterialFieldProgramName:
		return "program_name", true
	case model.MaterialFieldContactPhone:
		return "contact_phone", true
	default:
		return "", false
	}
}

// materialPresentSQL 返回"该资料项已填写"的 SQL 条件（空白视为未填写，
// 与 service.MaterialStatusOf 的 TrimSpace 口径一致）。
func materialPresentSQL(column string) string {
	return "TRIM(COALESCE(r." + column + ", '')) <> ''"
}

// materialCompleteSQL 返回"8 项资料全部填写"的 SQL 条件。
func materialCompleteSQL() string {
	columns := []string{"photo_path", "audio_path", "program_name", "teacher_name",
		"contact_phone", "express_address", "work_intro", "special_req"}
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, materialPresentSQL(column))
	}
	return "(" + strings.Join(parts, " AND ") + ")"
}
