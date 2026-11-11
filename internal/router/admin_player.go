package router

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/service"
)

// adminPlayerHandler 承载后台选手档案接口（开发计划 T16，API 文档 13.2）。
type adminPlayerHandler struct {
	svc *service.PlayerService
}

// registerAdminPlayerRoutes 注册 /api/v1/admin/players 下的接口。
// 档案合并为敏感操作，仅 admin / super_admin。
func registerAdminPlayerRoutes(group *gin.RouterGroup, svc *service.PlayerService) {
	h := &adminPlayerHandler{svc: svc}
	write := group.Group("", middleware.RequireRole(model.RoleSuperAdmin, model.RoleAdmin))
	write.POST("/players/:player_id/merge", h.merge)
}

// merge 把重复档案合并到保留档案（历史报名/成绩/积分随档案迁移）。
func (h *adminPlayerHandler) merge(c *gin.Context) {
	playerID, err := strconv.ParseInt(c.Param("player_id"), 10, 64)
	if err != nil || playerID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("选手 ID 不合法"))
		return
	}
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	var req model.MergePlayerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：target_player_id 为必填项"))
		return
	}
	resp, err := h.svc.Merge(c.Request.Context(), adminID, playerID, req.TargetPlayerID)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}
