package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// 赛季相关审计日志 action（admin_logs.action）。
const (
	adminActionSeasonCreate = "admin.season.create"
	adminActionSeasonUpdate = "admin.season.update"
)

// seasonGroupRules 是 seasons.group_rules 的 JSON 结构（系统级组别年龄规则，只读复制）。
type seasonGroupRules struct {
	AgeCutoff string `json:"age_cutoff"`
	Levels    []struct {
		Level         string `json:"level"`
		BirthYearFrom int    `json:"birth_year_from"`
		BirthYearTo   int    `json:"birth_year_to"`
	} `json:"levels"`
}

// SeasonService 负责赛季管理（开发计划 T6：简化版 seasons CRUD）。
//
// 约束：全系统同时最多一个 active 赛季；active 赛季才能挂赛事；
// 状态只按 upcoming → active → archived 向前流转，archived 不可再变更。
type SeasonService struct {
	stores *store.Store
}

// NewSeasonService 构造赛季服务。
func NewSeasonService(stores *store.Store) *SeasonService {
	return &SeasonService{stores: stores}
}

// List 返回全部赛季（按 id 倒序）。
func (s *SeasonService) List(ctx context.Context) ([]model.AdminSeasonItem, error) {
	list, err := s.stores.Seasons.List(ctx)
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询赛季失败，请稍后重试")
	}
	items := make([]model.AdminSeasonItem, 0, len(list))
	for _, season := range list {
		items = append(items, model.NewAdminSeasonItem(season))
	}
	return items, nil
}

// Create 创建赛季（恒为 upcoming，group_rules 从系统设置复制）。
//
// 错误码：参数非法（名称空/日期非 YYYY-MM-DD/起始晚于结束）→ 40001；
// 系统缺少 group_rules 配置 → 50001。
func (s *SeasonService) Create(ctx context.Context, adminID int64, req model.AdminSeasonRequest) (*model.AdminSeasonItem, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, httpx.ErrBadRequest("赛季名称不能为空")
	}
	if err := validateSeasonDates(req.StartDate, req.EndDate); err != nil {
		return nil, err
	}

	rules, err := s.systemGroupRules(ctx)
	if err != nil {
		return nil, err
	}

	var id int64
	if err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = s.stores.Seasons.Insert(ctx, tx, store.SeasonParams{
			Name:       name,
			StartDate:  emptyToNil(req.StartDate),
			EndDate:    emptyToNil(req.EndDate),
			GroupRules: rules,
		})
		if err != nil {
			return err
		}
		return adminLog(ctx, s.stores, tx, &adminID, adminActionSeasonCreate, strPtr("season"), &id,
			map[string]any{"name": name})
	}); err != nil {
		return nil, httpx.ErrInternal(err, "创建赛季失败，请稍后重试")
	}

	season, err := s.stores.Seasons.Get(ctx, id)
	if err != nil {
		return nil, httpx.ErrInternal(err, "创建赛季失败，请稍后重试")
	}
	item := model.NewAdminSeasonItem(*season)
	return &item, nil
}

// Update 编辑赛季（名称/日期/状态流转）。
//
// 错误码：赛季不存在 → 40401；已归档赛季拒绝任何变更 → 40901；
// 状态值非法 → 40001；状态流转非法（回退）→ 40901；
// 激活时已有其他 active 赛季 → 40901（部分唯一索引兜底并发）；日期非法 → 40001。
func (s *SeasonService) Update(ctx context.Context, adminID, id int64, req model.AdminSeasonRequest) (*model.AdminSeasonItem, error) {
	season, err := s.stores.Seasons.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrNotFound("赛季不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询赛季失败，请稍后重试")
	}
	// 归档为终止态：任何字段变更（含只改名称/日期、幂等 status）一律拒绝（审核 #9）。
	if season.Status == model.SeasonStatusArchived {
		return nil, httpx.ErrConflict("赛季已归档，不能再变更")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, httpx.ErrBadRequest("赛季名称不能为空")
	}
	if req.Status != "" &&
		req.Status != model.SeasonStatusUpcoming &&
		req.Status != model.SeasonStatusActive &&
		req.Status != model.SeasonStatusArchived {
		return nil, httpx.ErrBadRequest("赛季状态非法，应为 upcoming / active / archived")
	}

	startDate := req.StartDate
	if startDate == "" && season.StartDate != nil {
		startDate = *season.StartDate
	}
	endDate := req.EndDate
	if endDate == "" && season.EndDate != nil {
		endDate = *season.EndDate
	}
	if err := validateSeasonDates(startDate, endDate); err != nil {
		return nil, err
	}

	// 状态流转是否发生；激活走 Activate（CAS + 部分唯一索引兜底），其余走 patch。
	activating := false
	targetStatus := req.Status
	if targetStatus != "" && targetStatus != season.Status {
		if !validSeasonTransition(season.Status, targetStatus) {
			return nil, httpx.ErrConflict(fmt.Sprintf("赛季状态不允许从 %s 变更为 %s", season.Status, targetStatus))
		}
		if targetStatus == model.SeasonStatusActive {
			// 预检只为友好报错；真正的并发兜底是 uq_seasons_single_active 部分唯一索引。
			exists, err := s.stores.Seasons.HasOtherActive(ctx, id)
			if err != nil {
				return nil, httpx.ErrInternal(err, "查询赛季失败，请稍后重试")
			}
			if exists {
				return nil, httpx.ErrConflict("已存在进行中的赛季，请先归档当前赛季")
			}
			activating = true
		}
	} else {
		targetStatus = ""
	}

	patch := modelSeasonPatch(req, name, targetStatus)
	if err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		if activating {
			if err := s.stores.Seasons.Activate(ctx, tx, id, season.Status); err != nil {
				return err
			}
		} else if err := s.stores.Seasons.Update(ctx, tx, id, patch); err != nil {
			return err
		}
		detail := map[string]any{}
		if targetStatus != "" {
			detail["status_from"] = season.Status
			detail["status_to"] = targetStatus
		}
		if patch.Name != nil {
			detail["name"] = *patch.Name
		}
		return adminLog(ctx, s.stores, tx, &adminID, adminActionSeasonUpdate, strPtr("season"), &id, detail)
	}); err != nil {
		if errors.Is(err, store.ErrConflict) {
			// Activate 的唯一索引/条件冲突：并发激活或状态已被并发改变。
			return nil, httpx.ErrConflict("已存在进行中的赛季，请先归档当前赛季")
		}
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.ErrNotFound("赛季不存在")
		}
		return nil, httpx.ErrInternal(err, "更新赛季失败，请稍后重试")
	}

	updated, err := s.stores.Seasons.Get(ctx, id)
	if err != nil {
		return nil, httpx.ErrInternal(err, "更新赛季失败，请稍后重试")
	}
	item := model.NewAdminSeasonItem(*updated)
	return &item, nil
}

// systemGroupRules 读取系统设置中的 group_rules 模板原文。
func (s *SeasonService) systemGroupRules(ctx context.Context) (string, error) {
	rules, err := s.stores.Settings.Get(ctx, "group_rules")
	if errors.Is(err, store.ErrNotFound) {
		return "", httpx.ErrInternal(err, "系统缺少组别规则配置，请联系管理员")
	}
	if err != nil {
		return "", httpx.ErrInternal(err, "查询系统配置失败，请稍后重试")
	}
	return rules, nil
}

// validateSeasonDates 校验 YYYY-MM-DD 格式与 start <= end。
func validateSeasonDates(start, end string) error {
	if start != "" {
		if _, err := utils.ParseDate(start); err != nil {
			return httpx.ErrBadRequest("赛季开始日期格式应为 YYYY-MM-DD")
		}
	}
	if end != "" {
		if _, err := utils.ParseDate(end); err != nil {
			return httpx.ErrBadRequest("赛季结束日期格式应为 YYYY-MM-DD")
		}
	}
	if start != "" && end != "" && start > end {
		return httpx.ErrBadRequest("赛季开始日期不能晚于结束日期")
	}
	return nil
}

// validSeasonTransition 只允许 upcoming → active → archived 向前流转（可跳级归档）。
func validSeasonTransition(from, to string) bool {
	rank := map[string]int{
		model.SeasonStatusUpcoming: 0,
		model.SeasonStatusActive:   1,
		model.SeasonStatusArchived: 2,
	}
	return rank[to] > rank[from]
}

// modelSeasonPatch 按请求构造更新字段：名称始终更新；日期仅在请求显式提供时更新
// （省略即保留原值，避免"只改状态"把已填日期清空）；状态变更时同步写 archived_at。
func modelSeasonPatch(req model.AdminSeasonRequest, name, targetStatus string) store.SeasonPatch {
	patch := store.SeasonPatch{Name: &name}
	if req.StartDate != "" {
		patch.StartDate = &req.StartDate
	}
	if req.EndDate != "" {
		patch.EndDate = &req.EndDate
	}
	if targetStatus != "" {
		patch.Status = &targetStatus
		if targetStatus == model.SeasonStatusArchived {
			now := utils.NowISO()
			patch.ArchivedAt = &now
		}
	}
	return patch
}

func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
