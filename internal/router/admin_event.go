package router

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/service"
)

// adminSeasonHandler 承载后台赛季管理接口（开发计划 T6：简化版 seasons CRUD）。
type adminSeasonHandler struct {
	svc *service.SeasonService
}

// registerAdminSeasonRoutes 注册 /api/v1/admin/seasons 下的接口。
// 全部需要 admin token；写操作仅 admin / super_admin（cs 只读 → 40301）。
func registerAdminSeasonRoutes(group *gin.RouterGroup, svc *service.SeasonService) {
	h := &adminSeasonHandler{svc: svc}
	seasons := group.Group("/seasons")
	seasons.GET("", h.list)
	write := seasons.Group("", middleware.RequireRole(model.RoleSuperAdmin, model.RoleAdmin))
	write.POST("", h.create)
	write.PUT("/:id", h.update)
}

// list 返回全部赛季（按 id 倒序）。
func (h *adminSeasonHandler) list(c *gin.Context) {
	list, err := h.svc.List(c.Request.Context())
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, gin.H{"list": list})
}

// create 创建赛季（恒为 upcoming，组别规则从系统设置复制）。
func (h *adminSeasonHandler) create(c *gin.Context) {
	var req model.AdminSeasonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：name 不能为空"))
		return
	}
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	season, err := h.svc.Create(c.Request.Context(), adminID, req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, season)
}

// update 编辑赛季（名称/日期/状态流转）。
func (h *adminSeasonHandler) update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("赛季 ID 非法"))
		return
	}
	var req model.AdminSeasonRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：name 不能为空"))
		return
	}
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	season, err := h.svc.Update(c.Request.Context(), adminID, id, req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, season)
}

// adminEventHandler 承载后台赛事管理接口（开发计划 T6，API 文档 10.1-10.5）。
type adminEventHandler struct {
	svc *service.EventService
}

// registerAdminEventRoutes 注册 /api/v1/admin/events 下的接口。
// 列表全部 admin 角色可读；写操作仅 admin / super_admin（cs 只读 → 40301）。
func registerAdminEventRoutes(group *gin.RouterGroup, svc *service.EventService) {
	h := &adminEventHandler{svc: svc}
	events := group.Group("/events")
	events.GET("", h.list)
	events.GET("/:id", h.get)
	write := events.Group("", middleware.RequireRole(model.RoleSuperAdmin, model.RoleAdmin))
	write.POST("", h.create)
	write.PUT("/:id", h.update)
	write.POST("/:id/publish", h.publish)
	write.POST("/:id/offline", h.offline)
	write.POST("/:id/close-registration", h.closeRegistration)
	write.PUT("/:id/groups/:gid/quota", h.adjustQuota)
	write.POST("/:id/groups/:gid/close", h.closeGroup)
	write.POST("/:id/groups/:gid/reopen", h.reopenGroup)
}

// list 赛事分页列表（status/level/keyword 筛选）。
func (h *adminEventHandler) list(c *gin.Context) {
	q := model.AdminEventListQuery{
		Status:   c.Query("status"),
		Level:    c.Query("level"),
		Keyword:  c.Query("keyword"),
		Page:     queryInt(c, "page", 1),
		PageSize: queryIntMax(c, "page_size", 20, 100),
	}
	resp, err := h.svc.List(c.Request.Context(), q)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// get 赛事详情（API 文档 10.1b）：完整字段 + 组别报名进度。
func (h *adminEventHandler) get(c *gin.Context) {
	id, ok := eventIDParam(c)
	if !ok {
		return
	}
	item, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, item)
}

// create 创建赛事（publish=true 时保存并发布）。
func (h *adminEventHandler) create(c *gin.Context) {
	var req model.AdminCreateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：name/season_id/level/venue/event_date/fee/timeline/groups 必填"))
		return
	}
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	resp, err := h.svc.Create(c.Request.Context(), adminID, req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// update 编辑赛事。
func (h *adminEventHandler) update(c *gin.Context) {
	id, ok := eventIDParam(c)
	if !ok {
		return
	}
	var req model.AdminUpdateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：name/level/venue/event_date/fee/timeline/groups 必填"))
		return
	}
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	resp, err := h.svc.Update(c.Request.Context(), adminID, id, req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// publish 发布赛事（draft/unpublished → registering）。
func (h *adminEventHandler) publish(c *gin.Context) {
	h.mutateEventStatus(c, h.svc.Publish)
}

// offline 下架赛事（非终态 → offline）。
func (h *adminEventHandler) offline(c *gin.Context) {
	h.mutateEventStatus(c, h.svc.Offline)
}

// closeRegistration 关闭报名（registering → reg_closed）。
func (h *adminEventHandler) closeRegistration(c *gin.Context) {
	h.mutateEventStatus(c, h.svc.CloseRegistration)
}

// mutateEventStatus 是三个无请求体的状态流转接口的公共处理。
func (h *adminEventHandler) mutateEventStatus(c *gin.Context, fn func(ctx context.Context, adminID, id int64) (*model.AdminEventMutationResponse, error)) {
	id, ok := eventIDParam(c)
	if !ok {
		return
	}
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	resp, err := fn(c.Request.Context(), adminID, id)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// adjustQuota 调整报名单元名额（只能调大）。
func (h *adminEventHandler) adjustQuota(c *gin.Context) {
	eventID, groupID, ok := eventGroupIDParams(c)
	if !ok {
		return
	}
	var req model.AdminGroupQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Quota <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：quota 必须为正整数"))
		return
	}
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	resp, err := h.svc.AdjustQuota(c.Request.Context(), adminID, eventID, groupID, req.Quota)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// closeGroup 关闭报名单元（已支付报名不受影响）。
func (h *adminEventHandler) closeGroup(c *gin.Context) {
	h.mutateGroupEnabled(c, false)
}

// reopenGroup 重新开放报名单元。
func (h *adminEventHandler) reopenGroup(c *gin.Context) {
	h.mutateGroupEnabled(c, true)
}

func (h *adminEventHandler) mutateGroupEnabled(c *gin.Context, enabled bool) {
	eventID, groupID, ok := eventGroupIDParams(c)
	if !ok {
		return
	}
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	resp, err := h.svc.SetGroupEnabled(c.Request.Context(), adminID, eventID, groupID, enabled)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// eventIDParam 解析路径参数 event_id；非法时直接 AbortError。
func eventIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("赛事 ID 非法"))
		return 0, false
	}
	return id, true
}

// eventGroupIDParams 解析路径参数 event_id 与 gid；非法时直接 AbortError。
func eventGroupIDParams(c *gin.Context) (eventID, groupID int64, ok bool) {
	eventID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || eventID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("赛事 ID 非法"))
		return 0, 0, false
	}
	groupID, err = strconv.ParseInt(c.Param("gid"), 10, 64)
	if err != nil || groupID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("组别 ID 非法"))
		return 0, 0, false
	}
	return eventID, groupID, true
}

// queryInt 解析非负整数 query 参数（缺省返回 def）。
func queryInt(c *gin.Context, name string, def int) int {
	v := c.Query(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return n
}

// queryIntMax 解析带上限的非负整数 query 参数。
func queryIntMax(c *gin.Context, name string, def, max int) int {
	n := queryInt(c, name, def)
	if n > max {
		return max
	}
	return n
}
