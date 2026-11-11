package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
)

// AdminMissingMaterialFilter 是资料催收名单的筛选与分页条件（API 文档 12.3）。
type AdminMissingMaterialFilter struct {
	EventID       int64
	Level         string
	DanceType     string
	MissingItem   string
	DeadlineState string // open / closed / 空
	Now           string // 全库统一时间格式，用于截止状态判断
	Page          int
	PageSize      int
}

// AdminMissingMaterialRow 是催收名单的一行（报名单 + 赛事/组别/选手/联系记录）。
type AdminMissingMaterialRow struct {
	Registration    model.Registration
	OrderNo         *string
	EventName       string
	Level           string
	PlayerNo        string
	PlayerName      string
	ContactPhone    *string
	MaterialCheckAt *string
	ContactedAt     *string
}

// ListMissing 分页查询资料未传齐名单：active 报名且 photo/audio/program_name
// 任一为空；可按赛事/组别/舞种/单项缺失/截止状态筛选。
func (s *RegistrationStore) ListMissing(ctx context.Context, f AdminMissingMaterialFilter) ([]AdminMissingMaterialRow, int64, error) {
	where := []string{
		"r.status = 'active'",
		"(NOT (" + materialPresentSQL("photo_path") + ") OR NOT (" +
			materialPresentSQL("audio_path") + ") OR NOT (" +
			materialPresentSQL("program_name") + "))",
	}
	args := make([]any, 0, 6)
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
	if column, ok := materialItemColumn(f.MissingItem); ok {
		where = append(where, "NOT ("+materialPresentSQL(column)+")")
	}
	switch f.DeadlineState {
	case "open":
		where = append(where, "(e.material_check_at IS NULL OR e.material_check_at > ?)")
		args = append(args, f.Now)
	case "closed":
		where = append(where, "(e.material_check_at IS NOT NULL AND e.material_check_at <= ?)")
		args = append(args, f.Now)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := s.db.sql.QueryRowContext(ctx,
		`SELECT COUNT(1) `+adminRegistrationFrom+` WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count missing materials: %w", err)
	}
	if total == 0 {
		return []AdminMissingMaterialRow{}, 0, nil
	}

	rows, err := s.db.sql.QueryContext(ctx, `
		SELECT `+adminRegistrationSelectColumns+`,
		       o.order_no, e.name, g.level, pl.player_no, pl.name,
		       COALESCE(r.contact_phone, u.phone), e.material_check_at, mc.contacted_at
		`+adminRegistrationFrom+`
		LEFT JOIN (
			SELECT registration_id, MAX(created_at) AS contacted_at
			FROM material_contacts GROUP BY registration_id
		) mc ON mc.registration_id = r.id
		WHERE `+cond+`
		ORDER BY e.id, g.level, r.dance_type, r.id
		LIMIT ? OFFSET ?`, append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query missing materials: %w", err)
	}
	defer rows.Close()

	list := make([]AdminMissingMaterialRow, 0, f.PageSize)
	for rows.Next() {
		var out AdminMissingMaterialRow
		reg := &out.Registration
		if err := rows.Scan(&reg.ID, &reg.EventID, &reg.GroupID, &reg.PlayerID, &reg.UserID,
			&reg.DanceType, &reg.Institution, &reg.Status, &reg.Source, &reg.CSTicketID,
			&reg.PhotoPath, &reg.AudioPath, &reg.ProgramName, &reg.TeacherName,
			&reg.ContactPhone, &reg.ExpressAddress, &reg.WorkIntro, &reg.SpecialReq,
			&reg.MaterialUpdatedAt, &reg.CreatedAt, &reg.UpdatedAt,
			&out.OrderNo, &out.EventName, &out.Level, &out.PlayerNo, &out.PlayerName,
			&out.ContactPhone, &out.MaterialCheckAt, &out.ContactedAt); err != nil {
			return nil, 0, fmt.Errorf("scan missing material: %w", err)
		}
		list = append(list, out)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate missing materials: %w", err)
	}
	return list, total, nil
}
