package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// 小程序赛事列表缓存：名额热度为近似值（API 文档 2.1 注意事项允许），
// 匿名请求短 TTL 缓存；带用户 token 的请求（"已报名"角标）绕过缓存保证实时。
const miniappListCacheTTL = 30 * time.Second

// PublicEventService 是小程序端赛事只读服务（开发计划 T7，API 文档 2.1/2.2）。
//
// 展示状态推导（与后台存储状态区分）：
//   - events.status = finished → finished；
//   - 其余已发布赛事按时间线推导：now < reg_start_at → upcoming；
//     reg_start_at <= now < reg_end_at → registering；now >= reg_end_at → reg_closed。
//
// 字段收敛：只暴露展示字段，不输出 score_rules/prize_rules、audio_review_token、
// season_id 等内部字段。
type PublicEventService struct {
	stores *store.Store
	now    func() time.Time

	mu       sync.Mutex
	cache    map[string]miniappListCacheEntry
	cacheTTL time.Duration
}

type miniappListCacheEntry struct {
	resp   *model.MiniappEventListResponse
	expiry time.Time
}

// NewPublicEventService 构造小程序赛事查询服务。
func NewPublicEventService(stores *store.Store) *PublicEventService {
	return &PublicEventService{
		stores:   stores,
		now:      time.Now,
		cache:    make(map[string]miniappListCacheEntry),
		cacheTTL: miniappListCacheTTL,
	}
}

// List 返回小程序赛事列表（API 文档 2.1）。
// userID 为 nil 表示匿名（registered_by_me 恒 false）。匿名响应走 30s 短缓存。
func (s *PublicEventService) List(ctx context.Context, q model.MiniappEventListQuery, userID *int64) (*model.MiniappEventListResponse, error) {
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	cacheKey := ""
	if userID == nil {
		cacheKey = q.Status + "|" + itoa(page) + "|" + itoa(pageSize)
		if resp, ok := s.cached(cacheKey); ok {
			return resp, nil
		}
	}

	events, err := s.stores.Events.ListPublished(ctx)
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
	}

	// 推导展示状态并按状态筛选（赛事数量小，内存过滤后分页）。
	now := s.now()
	items := make([]model.MiniappEventItem, 0, len(events))
	ids := make([]int64, 0, len(events))
	for i := range events {
		status := deriveMiniappStatus(&events[i], now)
		if q.Status != "" && q.Status != status {
			continue
		}
		ids = append(ids, events[i].ID)
		items = append(items, model.MiniappEventItem{
			ID:           events[i].ID,
			Name:         events[i].Name,
			Level:        events[i].Level,
			City:         events[i].City,
			Venue:        events[i].Venue,
			EventDate:    events[i].EventDate,
			CoverURL:     events[i].CoverPath,
			Fee:          events[i].Fee,
			Status:       status,
			RegEndAt:     events[i].RegEndAt,
			GroupSummary: []model.MiniappGroupSummary{},
		})
	}

	// 批量取组别汇总与"已报名"角标（各一次 IN 查询，防 N+1）。
	groupsByEvent, err := s.stores.Groups.ListByEvents(ctx, ids)
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
	}
	registeredByMe := map[int64]bool{}
	if userID != nil {
		registeredByMe, err = s.stores.Registrations.ActiveEventIDs(ctx, *userID, ids)
		if err != nil {
			return nil, httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
		}
	}
	for i := range items {
		for _, g := range groupsByEvent[items[i].ID] {
			items[i].GroupSummary = append(items[i].GroupSummary, model.MiniappGroupSummary{
				Level:      g.Level,
				DanceType:  g.DanceType,
				Registered: g.Taken,
				Quota:      g.Quota,
			})
		}
		items[i].RegisteredByMe = registeredByMe[items[i].ID]
	}

	total := int64(len(items))
	start := (page - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	resp := &model.MiniappEventListResponse{
		List:     items[start:end],
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}
	if cacheKey != "" {
		s.storeCache(cacheKey, resp)
	}
	return resp, nil
}

// Detail 返回小程序赛事详情（API 文档 2.2）。名额实时（quota - taken），
// 不缓存；draft/unpublished/offline 对已持有报名的用户同样 40401。
func (s *PublicEventService) Detail(ctx context.Context, id int64, userID *int64) (*model.MiniappEventDetail, error) {
	event, err := s.stores.Events.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !isPublishedVisible(event)) {
		return nil, httpx.ErrNotFound("赛事不存在或未发布")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
	}

	groups, err := s.stores.Groups.ListByEvent(ctx, id)
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
	}
	detailGroups := make([]model.MiniappEventGroup, 0, len(groups))
	for _, g := range groups {
		detailGroups = append(detailGroups, model.MiniappEventGroup{
			GroupID:       g.ID,
			Level:         g.Level,
			DanceType:     g.DanceType,
			BirthYearFrom: g.BirthYearFrom,
			BirthYearTo:   g.BirthYearTo,
			Quota:         g.Quota,
			Registered:    g.Taken,
			Remaining:     g.Quota - g.Taken,
			Enabled:       g.Enabled,
		})
	}

	status := deriveMiniappStatus(event, s.now())
	return &model.MiniappEventDetail{
		ID:        event.ID,
		Name:      event.Name,
		Level:     event.Level,
		City:      event.City,
		Venue:     event.Venue,
		EventDate: event.EventDate,
		CoverURL:  event.CoverPath,
		Fee:       event.Fee,
		Status:    status,
		Timeline: model.MiniappTimeline{
			RegStartAt:      event.RegStartAt,
			RegEndAt:        event.RegEndAt,
			LotteryAt:       event.LotteryAt,
			MaterialCheckAt: event.MaterialCheckAt,
			StartAt:         event.StartAt,
		},
		Groups:            detailGroups,
		RefundPolicy:      event.RefundPolicy,
		WorkDurationLimit: event.WorkDurationLimit,
		ProcessDesc:       event.ProcessDesc,
		HonorDesc:         event.HonorDesc,
	}, nil
}

// deriveMiniappStatus 推导小程序端展示状态（见类型注释）。
func deriveMiniappStatus(e *model.Event, now time.Time) string {
	if e.Status == model.EventStatusFinished {
		return model.MiniappStatusFinished
	}
	regStart := deref(e.RegStartAt)
	regEnd := deref(e.RegEndAt)
	if regStart != "" {
		if t, err := utils.ParseISO(regStart); err == nil && now.Before(t) {
			return model.MiniappStatusUpcoming
		}
	}
	if regEnd != "" {
		if t, err := utils.ParseISO(regEnd); err == nil && !now.Before(t) {
			return model.MiniappStatusRegClosed
		}
	}
	return model.MiniappStatusRegistering
}

// isPublishedVisible 返回赛事是否对小程序端可见（draft/unpublished/offline 不可见）。
func isPublishedVisible(e *model.Event) bool {
	if e == nil {
		return false
	}
	switch e.Status {
	case model.EventStatusDraft, model.EventStatusUnpublished, model.EventStatusOffline:
		return false
	}
	return true
}

func (s *PublicEventService) cached(key string) (*model.MiniappEventListResponse, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.cache[key]
	if !ok || s.now().After(entry.expiry) {
		delete(s.cache, key)
		return nil, false
	}
	return entry.resp, true
}

func (s *PublicEventService) storeCache(key string, resp *model.MiniappEventListResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = miniappListCacheEntry{resp: resp, expiry: s.now().Add(s.cacheTTL)}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
