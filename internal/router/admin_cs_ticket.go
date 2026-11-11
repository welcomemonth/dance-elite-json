package router

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/service"
)

// adminCSTicketHandler 承载后台客服工单接口（开发计划 T16，API 文档 13.3）。
type adminCSTicketHandler struct {
	svc *service.CSTicketService
}

// registerAdminCSTicketRoutes 注册 /api/v1/admin/cs-tickets 下的接口：
// 登记/列表 cs 与 admin 均可用（cs 只看自己登记的），流转仅 admin / super_admin。
func registerAdminCSTicketRoutes(group *gin.RouterGroup, svc *service.CSTicketService) {
	h := &adminCSTicketHandler{svc: svc}
	tickets := group.Group("/cs-tickets")
	tickets.GET("", h.list)
	tickets.POST("", h.create)
	write := tickets.Group("", middleware.RequireRole(model.RoleSuperAdmin, model.RoleAdmin))
	write.PUT("/:ticket_id", h.update)
}

// list 工单分页列表（type/status 筛选）。
func (h *adminCSTicketHandler) list(c *gin.Context) {
	adminID, role, ok := csTicketActor(c)
	if !ok {
		return
	}
	resp, err := h.svc.List(c.Request.Context(), adminID, role, model.AdminCSTicketListQuery{
		Type:     c.Query("type"),
		Status:   c.Query("status"),
		Page:     queryInt(c, "page", 1),
		PageSize: queryIntMax(c, "page_size", 20, 100),
	})
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// create 登记工单。
func (h *adminCSTicketHandler) create(c *gin.Context) {
	adminID, _, ok := csTicketActor(c)
	if !ok {
		return
	}
	var req model.CreateCSTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：type、phone、content 为必填项"))
		return
	}
	resp, err := h.svc.Create(c.Request.Context(), adminID, req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// update 流转工单（pending → processing → done）。
func (h *adminCSTicketHandler) update(c *gin.Context) {
	adminID, _, ok := csTicketActor(c)
	if !ok {
		return
	}
	ticketID, err := strconv.ParseInt(c.Param("ticket_id"), 10, 64)
	if err != nil || ticketID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("工单 ID 不合法"))
		return
	}
	var req model.UpdateCSTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：status 为必填项"))
		return
	}
	resp, err := h.svc.Update(c.Request.Context(), adminID, ticketID, req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// csTicketActor 取当前管理员 ID 与角色；未登录时直接 AbortError。
func csTicketActor(c *gin.Context) (int64, string, bool) {
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return 0, "", false
	}
	role, ok := middleware.GetAdminRole(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return 0, "", false
	}
	return adminID, role, true
}
