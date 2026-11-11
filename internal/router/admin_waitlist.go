package router

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/service"
)

// adminWaitlistHandler 承载后台候补队列接口（开发计划 T14，API 文档 12.1–12.2）。
type adminWaitlistHandler struct {
	svc *service.WaitlistService
}

// registerAdminWaitlistRoutes 注册 /api/v1/admin/waitlists 下的接口。
// 列表对 admin/super_admin/cs 全部可读；写操作仅 admin / super_admin（cs 只读 → 40301）。
func registerAdminWaitlistRoutes(group *gin.RouterGroup, svc *service.WaitlistService) {
	h := &adminWaitlistHandler{svc: svc}
	waitlists := group.Group("/waitlists")
	waitlists.GET("", h.list)
	write := waitlists.Group("", middleware.RequireRole(model.RoleSuperAdmin, model.RoleAdmin))
	write.POST("/:waitlist_id/notify", h.notify)
	write.POST("/:waitlist_id/remove", h.remove)
	write.POST("/:waitlist_id/convert", h.convert)
}

// list 候补队列分页列表（event_id/level/dance_type/status 筛选）。
func (h *adminWaitlistHandler) list(c *gin.Context) {
	eventID, ok := optionalInt64Query(c, "event_id")
	if !ok {
		httpx.AbortError(c, httpx.ErrBadRequest("event_id 不合法"))
		return
	}
	resp, err := h.svc.AdminList(c.Request.Context(), model.AdminWaitlistListQuery{
		EventID:   eventID,
		Level:     c.Query("level"),
		DanceType: c.Query("dance_type"),
		Status:    c.Query("status"),
		Page:      queryInt(c, "page", 1),
		PageSize:  queryIntMax(c, "page_size", 20, 100),
	})
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// notify 标记已通知（通知结果备注写入审计日志）。
func (h *adminWaitlistHandler) notify(c *gin.Context) {
	waitlistID, adminID, ok := waitlistActionIDs(c)
	if !ok {
		return
	}
	var req model.AdminWaitlistNotifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：note 为必填项"))
		return
	}
	resp, err := h.svc.AdminNotify(c.Request.Context(), adminID, waitlistID, req.Note)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// remove 移出队列（必填原因，二次确认由前端负责）。
func (h *adminWaitlistHandler) remove(c *gin.Context) {
	waitlistID, adminID, ok := waitlistActionIDs(c)
	if !ok {
		return
	}
	var req model.AdminWaitlistRemoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：reason 为必填项"))
		return
	}
	resp, err := h.svc.AdminRemove(c.Request.Context(), adminID, waitlistID, req.Reason)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// convert 手动关联已支付订单（置 converted）。
func (h *adminWaitlistHandler) convert(c *gin.Context) {
	waitlistID, adminID, ok := waitlistActionIDs(c)
	if !ok {
		return
	}
	var req model.AdminWaitlistConvertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：order_no 为必填项"))
		return
	}
	resp, err := h.svc.AdminConvert(c.Request.Context(), adminID, waitlistID, req.OrderNo)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// waitlistActionIDs 解析路径中的 waitlist_id 与当前管理员 ID；非法时直接 AbortError。
func waitlistActionIDs(c *gin.Context) (waitlistID, adminID int64, ok bool) {
	waitlistID, err := strconv.ParseInt(c.Param("waitlist_id"), 10, 64)
	if err != nil || waitlistID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("候补 ID 不合法"))
		return 0, 0, false
	}
	adminID, ok = middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return 0, 0, false
	}
	return waitlistID, adminID, true
}

// optionalInt64Query 解析可选的正整数 query 参数；缺省返回 0。
func optionalInt64Query(c *gin.Context, name string) (int64, bool) {
	v := c.Query(name)
	if v == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
