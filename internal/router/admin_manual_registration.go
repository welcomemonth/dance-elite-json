package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/service"
)

// adminManualRegistrationHandler 承载后台人工报名接口（开发计划 T16，API 文档 11.5）。
type adminManualRegistrationHandler struct {
	svc *service.ManualRegistrationService
}

// registerAdminManualRegistrationRoutes 注册 /api/v1/admin/manual-registrations。
// 人工报名为资金/建档操作，仅 admin / super_admin（cs 仅可登记工单 → 40301）。
func registerAdminManualRegistrationRoutes(group *gin.RouterGroup, svc *service.ManualRegistrationService) {
	h := &adminManualRegistrationHandler{svc: svc}
	write := group.Group("", middleware.RequireRole(model.RoleSuperAdmin, model.RoleAdmin))
	write.POST("/manual-registrations", h.create)
}

// create 新建人工报名（报名单 + 已支付订单 + 支付流水）。
func (h *adminManualRegistrationHandler) create(c *gin.Context) {
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	var req model.ManualRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：event_id、group_id、player_name、id_card_no、contact_phone、charge_type 为必填项"))
		return
	}
	resp, err := h.svc.Create(c.Request.Context(), adminID, req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}
