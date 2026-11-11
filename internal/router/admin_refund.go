package router

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/service"
)

// adminRefundHandler 承载后台退款工单接口（开发计划 T15，API 文档 11.4）。
type adminRefundHandler struct {
	svc *service.RefundService
}

// registerAdminRefundRoutes 注册 /api/v1/admin/refund-tickets 下的接口。
// 登记：客服/管理员均可（T15 计划）；同意/驳回：仅 admin / super_admin。
func registerAdminRefundRoutes(group *gin.RouterGroup, svc *service.RefundService) {
	h := &adminRefundHandler{svc: svc}
	tickets := group.Group("/refund-tickets")
	tickets.POST("", h.create)
	write := tickets.Group("", middleware.RequireRole(model.RoleSuperAdmin, model.RoleAdmin))
	write.POST("/:ticket_id/approve", h.approve)
	write.POST("/:ticket_id/reject", h.reject)
}

// create 后台代录退款工单。
func (h *adminRefundHandler) create(c *gin.Context) {
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	var req model.CreateRefundTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：order_no 与 reason 为必填项"))
		return
	}
	resp, err := h.svc.CreateTicket(c.Request.Context(), adminID, req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// approve 同意退款（资金操作，前端负责二次确认）。
func (h *adminRefundHandler) approve(c *gin.Context) {
	ticketID, adminID, ok := refundActionIDs(c)
	if !ok {
		return
	}
	var req model.ApproveRefundTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：amount 为必填项"))
		return
	}
	resp, err := h.svc.Approve(c.Request.Context(), adminID, ticketID, req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// reject 驳回退款（必填驳回原因）。
func (h *adminRefundHandler) reject(c *gin.Context) {
	ticketID, adminID, ok := refundActionIDs(c)
	if !ok {
		return
	}
	var req model.RejectRefundTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：reject_reason 为必填项"))
		return
	}
	resp, err := h.svc.Reject(c.Request.Context(), adminID, ticketID, req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// refundActionIDs 解析路径中的 ticket_id 与当前管理员 ID。
func refundActionIDs(c *gin.Context) (ticketID, adminID int64, ok bool) {
	ticketID, err := strconv.ParseInt(c.Param("ticket_id"), 10, 64)
	if err != nil || ticketID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("工单 ID 不合法"))
		return 0, 0, false
	}
	adminID, ok = middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return 0, 0, false
	}
	return ticketID, adminID, true
}
