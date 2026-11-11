package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// 赛事相关审计日志 action（admin_logs.action）。
const (
	adminActionEventCreate      = "admin.event.create"
	adminActionEventUpdate      = "admin.event.update"
	adminActionEventPublish     = "admin.event.publish"
	adminActionEventOffline     = "admin.event.offline"
	adminActionEventCloseReg    = "admin.event.close_registration"
	adminActionEventGroupQuota  = "admin.event_group.quota"
	adminActionEventGroupToggle = "admin.event_group.toggle"
)

// EventService 负责后台赛事与组别×舞种管理（开发计划 T6，API 文档 10.1-10.5）。
//
// 关键规则：
//   - 时间线 5 节点强制先后：reg_start < reg_end < lottery < material_check < start；
//   - 只有 active 赛季才能挂赛事；出生年份范围从赛季 group_rules 复制（系统规则只读）；
//   - 名额发布后只能调大；已发布赛事时间线、级别不可改（须下架 + 新建）；
//   - 状态机：publish 仅从 draft/unpublished → registering；close_registration 仅
//     registering → reg_closed；offline 可从 draft/unpublished/registering/reg_closed
//     进入且为终止态。全部流转写 admin_logs。
type EventService struct {
	stores *store.Store
}

// NewEventService 构造赛事服务。
func NewEventService(stores *store.Store) *EventService {
	return &EventService{stores: stores}
}

// groupQuotaChange 是一次名额调整计划（编辑赛事/调名额接口共用校验）。
type groupQuotaChange struct {
	groupID int64
	to      int
}

// List 返回后台赛事分页列表（含各组别报名进度，API 文档 10.1）。
func (s *EventService) List(ctx context.Context, q model.AdminEventListQuery) (*model.AdminEventListResponse, error) {
	events, total, err := s.stores.Events.List(ctx, store.EventFilter{
		Status:   q.Status,
		Level:    q.Level,
		Keyword:  q.Keyword,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询赛事失败，请稍后重试")
	}

	// 批量取组别（一次 IN 查询内存归组），避免逐赛事查询的 N+1。
	ids := make([]int64, 0, len(events))
	for i := range events {
		ids = append(ids, events[i].ID)
	}
	groupsByEvent, err := s.stores.Groups.ListByEvents(ctx, ids)
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询赛事失败，请稍后重试")
	}

	items := make([]model.AdminEventItem, 0, len(events))
	for i := range events {
		items = append(items, model.NewAdminEventItem(events[i], groupProgress(groupsByEvent[events[i].ID])))
	}

	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	return &model.AdminEventListResponse{
		List:     items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// Get 返回单个赛事详情（含组别进度，API 文档 10.1b）。
func (s *EventService) Get(ctx context.Context, id int64) (*model.AdminEventItem, error) {
	event, err := s.stores.Events.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrNotFound("赛事不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询赛事失败，请稍后重试")
	}
	groups, err := s.stores.Groups.ListByEvent(ctx, id)
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询赛事失败，请稍后重试")
	}
	item := model.NewAdminEventItem(*event, groupProgress(groups))
	return &item, nil
}

// groupProgress 把组别实体转为响应视图（taken 即已报名数）。
func groupProgress(groups []model.EventGroup) []model.AdminEventGroupProgress {
	progress := make([]model.AdminEventGroupProgress, 0, len(groups))
	for _, g := range groups {
		progress = append(progress, model.AdminEventGroupProgress{
			GroupID:       g.ID,
			Level:         g.Level,
			DanceType:     g.DanceType,
			BirthYearFrom: g.BirthYearFrom,
			BirthYearTo:   g.BirthYearTo,
			Quota:         g.Quota,
			Registered:    g.Taken,
			Enabled:       g.Enabled,
		})
	}
	return progress
}

// Create 创建赛事（可选保存并发布，API 文档 10.2）。
//
// 错误码：参数/时间线/组别非法 → 40001；赛季不存在 → 40401；
// 赛季非 active → 40901；publish 校验未通过 → 40001。
func (s *EventService) Create(ctx context.Context, adminID int64, req model.AdminCreateEventRequest) (*model.AdminEventMutationResponse, error) {
	season, err := s.activeSeason(ctx, req.SeasonID)
	if err != nil {
		return nil, err
	}

	timeline, err := validateTimeline(req.Timeline)
	if err != nil {
		return nil, err
	}
	if err := validateEventBasics(req.Name, req.Level, req.EventDate, req.Fee); err != nil {
		return nil, err
	}

	// 出生年份范围按组别从赛季规则复制；同时检查请求内组合重复。
	rules, err := parseGroupRules(season.GroupRules)
	if err != nil {
		return nil, httpx.ErrInternal(err, "赛季组别规则配置异常，请联系管理员")
	}
	groupParams, err := buildGroupParams(req.Groups, rules)
	if err != nil {
		return nil, err
	}

	scoreRules, prizeRules, refundPolicy, err := s.eventDefaults(ctx, req.ScoreRules, req.PrizeRules, req.RefundPolicy)
	if err != nil {
		return nil, err
	}

	status := model.EventStatusDraft
	if req.Publish {
		status = model.EventStatusRegistering
	}

	params := store.EventParams{
		SeasonID:          season.ID,
		Name:              strings.TrimSpace(req.Name),
		Level:             req.Level,
		City:              emptyToNil(strings.TrimSpace(req.City)),
		Venue:             emptyToNil(strings.TrimSpace(req.Venue)),
		EventDate:         emptyToNil(req.EventDate),
		Fee:               *req.Fee,
		RegStartAt:        &timeline[0],
		RegEndAt:          &timeline[1],
		LotteryAt:         &timeline[2],
		MaterialCheckAt:   &timeline[3],
		StartAt:           &timeline[4],
		Status:            status,
		ScoreRules:        scoreRules,
		PrizeRules:        prizeRules,
		RefundPolicy:      refundPolicy,
		WorkDurationLimit: emptyToNil(req.WorkDurationLimit),
		ProcessDesc:       emptyToNil(req.ProcessDesc),
		HonorDesc:         emptyToNil(req.HonorDesc),
		ExcludeFromPrize:  req.ExcludeFromPrize,
	}

	var id int64
	if err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = s.stores.Events.Insert(ctx, tx, params)
		if err != nil {
			return err
		}
		for _, gp := range groupParams {
			gp.EventID = id
			if _, err := s.stores.Groups.Insert(ctx, tx, gp); err != nil {
				return err
			}
		}
		return adminLog(ctx, s.stores, tx, &adminID, adminActionEventCreate, strPtr("event"), &id,
			map[string]any{"name": params.Name, "publish": req.Publish})
	}); err != nil {
		return nil, httpx.ErrInternal(err, "创建赛事失败，请稍后重试")
	}

	return &model.AdminEventMutationResponse{EventID: id, Status: status}, nil
}

// Update 编辑赛事（API 文档 10.3）。
//
// 草稿/未发布可全量编辑（含新增组别组合）；已发布（registering/reg_closed/ongoing）
// 时间线与级别不可改、名额只能调大、不可增删组别；finished/offline 不可编辑（41012）。
func (s *EventService) Update(ctx context.Context, adminID, id int64, req model.AdminUpdateEventRequest) (*model.AdminEventMutationResponse, error) {
	event, err := s.stores.Events.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrNotFound("赛事不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询赛事失败，请稍后重试")
	}
	if event.Status == model.EventStatusFinished || event.Status == model.EventStatusOffline {
		return nil, httpx.ErrEventStateNotAllowed("赛事已结束或已下架，不可编辑")
	}
	published := event.Status == model.EventStatusRegistering ||
		event.Status == model.EventStatusRegClosed || event.Status == model.EventStatusOngoing

	timeline, err := validateTimeline(req.Timeline)
	if err != nil {
		return nil, err
	}
	if err := validateEventBasics(req.Name, req.Level, req.EventDate, req.Fee); err != nil {
		return nil, err
	}
	if published {
		if req.Level != event.Level {
			return nil, httpx.ErrBadRequest("已发布赛事不可修改级别，请下架后重新创建")
		}
		if timelineDiffers(timeline, event) {
			return nil, httpx.ErrBadRequest("已发布赛事不可修改时间线，请下架后重新创建")
		}
	}

	// 组别变更计划：已发布仅允许已有组别调大名额；草稿还允许新增组合。
	plan, err := s.planGroupChanges(ctx, event, req.Groups, published)
	if err != nil {
		return nil, err
	}

	patch := store.EventPatch{
		Name:              ptrOf(strings.TrimSpace(req.Name)),
		Level:             ptrOf(req.Level),
		City:              ptrOf(strings.TrimSpace(req.City)),
		Venue:             ptrOf(strings.TrimSpace(req.Venue)),
		EventDate:         ptrOf(req.EventDate),
		Fee:               req.Fee,
		RegStartAt:        &timeline[0],
		RegEndAt:          &timeline[1],
		LotteryAt:         &timeline[2],
		MaterialCheckAt:   &timeline[3],
		StartAt:           &timeline[4],
		ScoreRules:        ptrOf(strings.TrimSpace(req.ScoreRules)),
		PrizeRules:        ptrOf(strings.TrimSpace(req.PrizeRules)),
		RefundPolicy:      ptrOf(strings.TrimSpace(req.RefundPolicy)),
		WorkDurationLimit: ptrOf(strings.TrimSpace(req.WorkDurationLimit)),
		ProcessDesc:       ptrOf(strings.TrimSpace(req.ProcessDesc)),
		HonorDesc:         ptrOf(strings.TrimSpace(req.HonorDesc)),
		ExcludeFromPrize:  &req.ExcludeFromPrize,
	}

	if err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.stores.Events.Update(ctx, tx, id, patch); err != nil {
			return err
		}
		for _, qc := range plan.quotaChanges {
			// 已发布赛事只能调大（UpdateQuota 条件兜底）；草稿可增可减（SetQuota）。
			var err error
			if published {
				err = s.stores.Groups.UpdateQuota(ctx, tx, qc.groupID, qc.to)
			} else {
				err = s.stores.Groups.SetQuota(ctx, tx, qc.groupID, qc.to)
			}
			if err != nil {
				return err
			}
		}
		for _, gp := range plan.newGroups {
			gp.EventID = id
			if _, err := s.stores.Groups.Insert(ctx, tx, gp); err != nil {
				return err
			}
		}
		return adminLog(ctx, s.stores, tx, &adminID, adminActionEventUpdate, strPtr("event"), &id,
			map[string]any{"name": *patch.Name})
	}); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return nil, httpx.ErrBadRequest("名额只能调大，且不能小于已报名人数")
		}
		return nil, httpx.ErrInternal(err, "更新赛事失败，请稍后重试")
	}

	return &model.AdminEventMutationResponse{EventID: id, Status: event.Status}, nil
}

// Publish 发布赛事：draft/unpublished → registering（API 文档 10.4）。
// 校验时间线完整且至少有一个 enabled 组别（COUNT 与流转、审计同一事务，
// 避免"校验后、流转前组别被关光"的窗口）；非法状态 → 41012。
func (s *EventService) Publish(ctx context.Context, adminID, id int64) (*model.AdminEventMutationResponse, error) {
	event, err := s.stores.Events.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrNotFound("赛事不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询赛事失败，请稍后重试")
	}
	if event.Status != model.EventStatusDraft && event.Status != model.EventStatusUnpublished {
		return nil, httpx.ErrEventStateNotAllowed("当前赛事状态不允许发布")
	}

	timeline := timelineOf(event)
	for i, v := range timeline {
		if v == "" {
			names := []string{"报名开始", "报名截止", "抽签时间", "资料审核", "比赛开始"}
			return nil, httpx.ErrBadRequest("发布前必须填写" + names[i] + "时间")
		}
	}

	err = s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		enabled, err := s.stores.Groups.CountEnabledTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if enabled == 0 {
			return httpx.ErrBadRequest("发布前至少需要一个启用状态的组别")
		}
		if err := s.stores.Events.TransitionStatus(ctx, tx, id,
			[]string{model.EventStatusDraft, model.EventStatusUnpublished},
			model.EventStatusRegistering); err != nil {
			return err
		}
		return adminLog(ctx, s.stores, tx, &adminID, adminActionEventPublish, strPtr("event"), &id,
			map[string]any{"status_from": event.Status, "status_to": model.EventStatusRegistering})
	})
	if err != nil {
		return nil, mapTransitionErr(err)
	}
	return &model.AdminEventMutationResponse{EventID: id, Status: model.EventStatusRegistering}, nil
}

// Offline 下架赛事：draft/unpublished/registering/reg_closed → offline（API 文档 10.4）。
// 下架后小程序端不可见不可报，已支付报名不受影响；offline 为终止态。
func (s *EventService) Offline(ctx context.Context, adminID, id int64) (*model.AdminEventMutationResponse, error) {
	event, err := s.stores.Events.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrNotFound("赛事不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询赛事失败，请稍后重试")
	}

	if err := s.transitionWithLog(ctx, adminID, id,
		[]string{model.EventStatusDraft, model.EventStatusUnpublished,
			model.EventStatusRegistering, model.EventStatusRegClosed},
		model.EventStatusOffline, adminActionEventOffline, event.Status); err != nil {
		return nil, err
	}
	return &model.AdminEventMutationResponse{EventID: id, Status: model.EventStatusOffline}, nil
}

// CloseRegistration 关闭报名：registering → reg_closed（开发计划 T6 状态机）。
func (s *EventService) CloseRegistration(ctx context.Context, adminID, id int64) (*model.AdminEventMutationResponse, error) {
	event, err := s.stores.Events.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrNotFound("赛事不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询赛事失败，请稍后重试")
	}

	if err := s.transitionWithLog(ctx, adminID, id,
		[]string{model.EventStatusRegistering},
		model.EventStatusRegClosed, adminActionEventCloseReg, event.Status); err != nil {
		return nil, err
	}
	return &model.AdminEventMutationResponse{EventID: id, Status: model.EventStatusRegClosed}, nil
}

// AdjustQuota 调整某报名单元名额（只能调大，API 文档 10.5）。
func (s *EventService) AdjustQuota(ctx context.Context, adminID, eventID, groupID int64, quota int) (*model.AdminGroupQuotaResponse, error) {
	event, group, err := s.eventAndGroup(ctx, eventID, groupID)
	if err != nil {
		return nil, err
	}
	if event.Status == model.EventStatusFinished || event.Status == model.EventStatusOffline {
		return nil, httpx.ErrEventStateNotAllowed("赛事已结束或已下架，不可调整名额")
	}
	if quota <= group.Quota {
		return nil, httpx.ErrBadRequest(
			fmt.Sprintf("名额只能调大：当前名额 %d，已报名 %d", group.Quota, group.Taken))
	}

	var registered int
	if err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.stores.Groups.UpdateQuota(ctx, tx, groupID, quota); err != nil {
			return err
		}
		registered = group.Taken
		return adminLog(ctx, s.stores, tx, &adminID, adminActionEventGroupQuota, strPtr("event_group"), &groupID,
			map[string]any{"event_id": eventID, "quota_from": group.Quota, "quota_to": quota})
	}); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return nil, httpx.ErrBadRequest("名额只能调大，且不能小于已报名人数")
		}
		return nil, httpx.ErrInternal(err, "调整名额失败，请稍后重试")
	}

	return &model.AdminGroupQuotaResponse{
		GroupID:    int(group.ID),
		Quota:      quota,
		Registered: registered,
		Remaining:  quota - registered,
	}, nil
}

// SetGroupEnabled 关闭/重新开放报名单元（不影响已支付报名，API 文档 10.5）。
func (s *EventService) SetGroupEnabled(ctx context.Context, adminID, eventID, groupID int64, enabled bool) (*model.AdminGroupEnabledResponse, error) {
	event, group, err := s.eventAndGroup(ctx, eventID, groupID)
	if err != nil {
		return nil, err
	}
	if event.Status == model.EventStatusFinished || event.Status == model.EventStatusOffline {
		return nil, httpx.ErrEventStateNotAllowed("赛事已结束或已下架，不可操作组别")
	}

	if err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.stores.Groups.SetEnabled(ctx, tx, groupID, enabled); err != nil {
			return err
		}
		return adminLog(ctx, s.stores, tx, &adminID, adminActionEventGroupToggle, strPtr("event_group"), &groupID,
			map[string]any{"event_id": eventID, "enabled_from": group.Enabled, "enabled_to": enabled})
	}); err != nil {
		return nil, httpx.ErrInternal(err, "操作组别失败，请稍后重试")
	}

	return &model.AdminGroupEnabledResponse{GroupID: int(group.ID), Enabled: enabled}, nil
}

// groupChangePlan 是编辑赛事时组别变更的校验结果：调名额 + 新增组合。
type groupChangePlan struct {
	quotaChanges []groupQuotaChange
	newGroups    []store.EventGroupParams
}

// planGroupChanges 校验编辑请求中的组别输入并生成变更计划。
// published=true 时只允许已有组别调大名额；否则还允许新增组合（出生年份从赛季规则复制）。
func (s *EventService) planGroupChanges(ctx context.Context, event *model.Event, inputs []model.AdminEventGroupInput, published bool) (*groupChangePlan, error) {
	existing, err := s.stores.Groups.ListByEvent(ctx, event.ID)
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询组别失败，请稍后重试")
	}
	byID := make(map[int64]model.EventGroup, len(existing))
	for _, g := range existing {
		byID[g.ID] = g
	}

	// 新增组合需要出生年份规则：草稿才可能走到这里，赛季规则一次性查出复用。
	var rules *seasonGroupRules
	loadRules := func() (*seasonGroupRules, error) {
		if rules == nil {
			r, err := s.seasonGroupRules(ctx, event.SeasonID)
			if err != nil {
				return nil, err
			}
			rules = r
		}
		return rules, nil
	}

	var plan groupChangePlan
	seen := make(map[string]bool, len(inputs))
	byCombo := make(map[string]model.EventGroup, len(existing))
	for _, g := range existing {
		byCombo[g.Level+"/"+g.DanceType] = g
	}
	for _, in := range inputs {
		// 按 group_id 或 组别+舞种 定位已有报名单元（API 文档 10.3 允许两种传法）。
		var existingGroup *model.EventGroup
		if in.GroupID != nil {
			g, ok := byID[*in.GroupID]
			if !ok {
				return nil, httpx.ErrNotFound("组别不存在")
			}
			existingGroup = &g
		} else if err := validateGroupLevelDance(in.Level, in.DanceType, in.Quota); err != nil {
			return nil, err
		} else if g, ok := byCombo[in.Level+"/"+in.DanceType]; ok {
			existingGroup = &g
		}

		if existingGroup != nil {
			g := existingGroup
			key := g.Level + "/" + g.DanceType
			if seen[key] {
				return nil, httpx.ErrBadRequest("组别配置重复：" + key)
			}
			seen[key] = true
			if in.Quota <= 0 {
				return nil, httpx.ErrBadRequest("名额必须为正整数")
			}
			if in.Quota != g.Quota {
				if published && in.Quota < g.Quota {
					return nil, httpx.ErrBadRequest(
						fmt.Sprintf("名额只能调大：%s 当前名额 %d", key, g.Quota))
				}
				plan.quotaChanges = append(plan.quotaChanges, groupQuotaChange{
					groupID: g.ID, to: in.Quota,
				})
			}
			continue
		}

		// 无匹配已有组别 → 新增组合，仅草稿/未发布允许。
		if published {
			return nil, httpx.ErrBadRequest("已发布赛事不可新增组别")
		}
		key := in.Level + "/" + in.DanceType
		if seen[key] {
			return nil, httpx.ErrBadRequest("组别配置重复：" + key)
		}
		seen[key] = true
		rl, err := loadRules()
		if err != nil {
			return nil, err
		}
		from, to, err := groupRuleOf(rl, in.Level)
		if err != nil {
			return nil, httpx.ErrInternal(err, "赛季组别规则配置异常，请联系管理员")
		}
		plan.newGroups = append(plan.newGroups, store.EventGroupParams{
			Level:         in.Level,
			DanceType:     in.DanceType,
			BirthYearFrom: from,
			BirthYearTo:   to,
			Quota:         in.Quota,
		})
	}
	return &plan, nil
}

// seasonGroupRules 读取赛事所属赛季的 group_rules。
func (s *EventService) seasonGroupRules(ctx context.Context, seasonID int64) (*seasonGroupRules, error) {
	season, err := s.stores.Seasons.Get(ctx, seasonID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrNotFound("赛季不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询赛季失败，请稍后重试")
	}
	rules, err := parseGroupRules(season.GroupRules)
	if err != nil {
		return nil, httpx.ErrInternal(err, "赛季组别规则配置异常，请联系管理员")
	}
	return rules, nil
}

// activeSeason 校验并返回赛事所属赛季：不存在 → 40401；非 active → 40901。
func (s *EventService) activeSeason(ctx context.Context, seasonID int64) (*model.Season, error) {
	season, err := s.stores.Seasons.Get(ctx, seasonID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrNotFound("赛季不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询赛季失败，请稍后重试")
	}
	if season.Status != model.SeasonStatusActive {
		return nil, httpx.ErrConflict("只有进行中的赛季才能创建赛事")
	}
	return season, nil
}

// eventAndGroup 查询赛事与其下指定组别，校验归属关系。
func (s *EventService) eventAndGroup(ctx context.Context, eventID, groupID int64) (*model.Event, *model.EventGroup, error) {
	event, err := s.stores.Events.Get(ctx, eventID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, httpx.ErrNotFound("赛事不存在")
	}
	if err != nil {
		return nil, nil, httpx.ErrInternal(err, "查询赛事失败，请稍后重试")
	}
	group, err := s.stores.Groups.Get(ctx, groupID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, httpx.ErrNotFound("组别不存在")
	}
	if err != nil {
		return nil, nil, httpx.ErrInternal(err, "查询组别失败，请稍后重试")
	}
	if group.EventID != event.ID {
		return nil, nil, httpx.ErrNotFound("组别不存在")
	}
	return event, group, nil
}

// transitionWithLog 在单一事务内完成状态条件流转 + admin_logs 审计，
// 避免"流转已提交、日志写失败"产生无审计的既成事实（审核 #3）。
// fromStatus 为日志记录的前置状态（调用前已读取）。
func (s *EventService) transitionWithLog(ctx context.Context, adminID, id int64, from []string, to, action, fromStatus string) error {
	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.stores.Events.TransitionStatus(ctx, tx, id, from, to); err != nil {
			return err
		}
		return adminLog(ctx, s.stores, tx, &adminID, action, strPtr("event"), &id,
			map[string]any{"status_from": fromStatus, "status_to": to})
	})
	return mapTransitionErr(err)
}

// mapTransitionErr 把流转事务的错误映射为对外业务错误：
// 状态条件未命中 → 41012；赛事不存在 → 40401；已是 BizError 原样返回；其余 → 50001。
func mapTransitionErr(err error) error {
	if err == nil {
		return nil
	}
	var biz *httpx.BizError
	if errors.As(err, &biz) {
		return err
	}
	if errors.Is(err, store.ErrConflict) {
		return httpx.ErrEventStateNotAllowed("当前赛事状态不允许该操作")
	}
	if errors.Is(err, store.ErrNotFound) {
		return httpx.ErrNotFound("赛事不存在")
	}
	return httpx.ErrInternal(err, "操作失败，请稍后重试")
}

// validateTimeline 校验时间线五节点：全部必填、ISO 8601 格式、严格递增。
// 入库统一规范为 +08:00（Asia/Shanghai）表示：DB 的时间线 CHECK 是字符串比较，
// 不规范化会让"语义合法但偏移不同"的时间（如 +00:00）被约束拒绝变成 500。
// 返回按 reg_start/reg_end/lottery/material_check/start 顺序的规范化字符串数组。
func validateTimeline(t model.EventTimeline) ([5]string, error) {
	var out [5]string
	raw := []string{t.RegStartAt, t.RegEndAt, t.LotteryAt, t.MaterialCheckAt, t.StartAt}
	names := []string{"报名开始", "报名截止", "抽签时间", "资料审核", "比赛开始"}
	var times [5]time.Time
	for i, v := range raw {
		v = strings.TrimSpace(v)
		if v == "" {
			return out, httpx.ErrBadRequest("时间线缺少" + names[i] + "时间")
		}
		parsed, err := utils.ParseISO(v)
		if err != nil {
			return out, httpx.ErrBadRequest(names[i] + "时间格式应为 ISO 8601（如 2027-01-01T09:00:00+08:00）")
		}
		out[i] = parsed.In(utils.ChinaLoc).Format("2006-01-02T15:04:05-07:00")
		times[i] = parsed
	}
	for i := 0; i < 4; i++ {
		if !times[i].Before(times[i+1]) {
			return out, httpx.ErrBadRequest("时间线顺序错误：" + names[i] + "必须早于" + names[i+1])
		}
	}
	return out, nil
}

// timelineOf 从赛事实体取时间线（publish 校验用；创建/编辑路径已保证非空）。
func timelineOf(e *model.Event) [5]string {
	return [5]string{deref(e.RegStartAt), deref(e.RegEndAt), deref(e.LotteryAt),
		deref(e.MaterialCheckAt), deref(e.StartAt)}
}

// timelineDiffers 比较请求时间线与赛事已存时间线是否一致（已发布赛事不可改）。
func timelineDiffers(t [5]string, e *model.Event) bool {
	return t[0] != deref(e.RegStartAt) || t[1] != deref(e.RegEndAt) ||
		t[2] != deref(e.LotteryAt) || t[3] != deref(e.MaterialCheckAt) ||
		t[4] != deref(e.StartAt)
}

// validateEventBasics 校验名称/级别/比赛日期/报名费。
func validateEventBasics(name, level, eventDate string, fee *int64) error {
	if strings.TrimSpace(name) == "" {
		return httpx.ErrBadRequest("赛事名称不能为空")
	}
	switch level {
	case model.EventLevelGradeA, model.EventLevelSuper, model.EventLevelFinal:
	default:
		return httpx.ErrBadRequest("赛事级别非法，应为 grade_a / super / final")
	}
	if _, err := utils.ParseDate(eventDate); err != nil {
		return httpx.ErrBadRequest("比赛日期格式应为 YYYY-MM-DD")
	}
	if fee == nil {
		return httpx.ErrBadRequest("报名费不能为空")
	}
	if *fee < 0 {
		return httpx.ErrBadRequest("报名费不能为负数")
	}
	return nil
}

// validateGroupLevelDance 校验组别/舞种枚举与名额正整数。
func validateGroupLevelDance(level, danceType string, quota int) error {
	switch level {
	case model.GroupLevelU11, model.GroupLevelU13, model.GroupLevelU15:
	default:
		return httpx.ErrBadRequest("组别非法，应为 U11 / U13 / U15")
	}
	switch danceType {
	case model.DanceTypeClassical, model.DanceTypeFolk:
	default:
		return httpx.ErrBadRequest("舞种非法，应为 classical / folk")
	}
	if quota <= 0 {
		return httpx.ErrBadRequest("名额必须为正整数")
	}
	return nil
}

// parseGroupRules 解析赛季 group_rules JSON。
func parseGroupRules(raw string) (*seasonGroupRules, error) {
	var rules seasonGroupRules
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, fmt.Errorf("parse group rules: %w", err)
	}
	if len(rules.Levels) == 0 {
		return nil, errors.New("group rules has no levels")
	}
	return &rules, nil
}

// groupRuleOf 返回某组别的出生年份范围；赛季规则缺该组别视为配置异常。
func groupRuleOf(rules *seasonGroupRules, level string) (from, to int, err error) {
	for _, l := range rules.Levels {
		if l.Level == level {
			return l.BirthYearFrom, l.BirthYearTo, nil
		}
	}
	return 0, 0, fmt.Errorf("group rules missing level %q", level)
}

// buildGroupParams 把创建请求中的组别输入转成插入参数（出生年份从赛季规则复制、查重）。
func buildGroupParams(inputs []model.AdminEventGroupInput, rules *seasonGroupRules) ([]store.EventGroupParams, error) {
	seen := make(map[string]bool, len(inputs))
	out := make([]store.EventGroupParams, 0, len(inputs))
	for _, in := range inputs {
		if in.GroupID != nil {
			return nil, httpx.ErrBadRequest("创建赛事时不能指定 group_id")
		}
		if err := validateGroupLevelDance(in.Level, in.DanceType, in.Quota); err != nil {
			return nil, err
		}
		key := in.Level + "/" + in.DanceType
		if seen[key] {
			return nil, httpx.ErrBadRequest("组别配置重复：" + key)
		}
		seen[key] = true
		from, to, err := groupRuleOf(rules, in.Level)
		if err != nil {
			return nil, httpx.ErrInternal(err, "赛季组别规则配置异常，请联系管理员")
		}
		out = append(out, store.EventGroupParams{
			Level:         in.Level,
			DanceType:     in.DanceType,
			BirthYearFrom: from,
			BirthYearTo:   to,
			Quota:         in.Quota,
		})
	}
	return out, nil
}

// eventDefaults 取积分/奖金规则模板与退费文案默认值。
// 请求显式提供时（需为合法 JSON）以请求为准，否则用系统默认模板。
func (s *EventService) eventDefaults(ctx context.Context, scoreRules, prizeRules, refundPolicy string) (*string, *string, *string, error) {
	score, err := s.ruleOrTemplate(ctx, "score_rules_template", scoreRules)
	if err != nil {
		return nil, nil, nil, err
	}
	prize, err := s.ruleOrTemplate(ctx, "prize_rules_template", prizeRules)
	if err != nil {
		return nil, nil, nil, err
	}
	policy := strings.TrimSpace(refundPolicy)
	if policy == "" {
		raw, err := s.stores.Settings.Get(ctx, "refund_policy")
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, nil, httpx.ErrInternal(err, "系统缺少退费政策配置，请联系管理员")
		}
		if err != nil {
			return nil, nil, nil, httpx.ErrInternal(err, "查询系统配置失败，请稍后重试")
		}
		var parsed string
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return nil, nil, nil, httpx.ErrInternal(err, "系统退费政策配置异常，请联系管理员")
		}
		policy = parsed
	}
	return score, prize, &policy, nil
}

// ruleOrTemplate 返回请求值（校验为合法 JSON）或系统模板原文。
func (s *EventService) ruleOrTemplate(ctx context.Context, templateKey, provided string) (*string, error) {
	if strings.TrimSpace(provided) != "" {
		if !json.Valid([]byte(provided)) {
			return nil, httpx.ErrBadRequest("规则配置不是合法 JSON")
		}
		v := provided
		return &v, nil
	}
	raw, err := s.stores.Settings.Get(ctx, templateKey)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrInternal(err, "系统缺少规则模板配置，请联系管理员")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "查询系统配置失败，请稍后重试")
	}
	return &raw, nil
}

func ptrOf[T any](v T) *T { return &v }

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
