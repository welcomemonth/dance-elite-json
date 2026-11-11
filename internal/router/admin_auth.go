package router

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/service"
)

// adminAuthHandler 承载后台认证相关接口（T5）。
type adminAuthHandler struct {
	svc *service.AdminAuthService
}

// registerAdminAuthRoutes 注册 /api/v1/admin/auth 下的接口。
//
// 登录限流 10 次/分钟/IP（外加服务层按用户名的连续失败锁定）；
// 修改密码需要 admin token。
func registerAdminAuthRoutes(group *gin.RouterGroup, svc *service.AdminAuthService, signer *token.Signer) {
	h := &adminAuthHandler{svc: svc}
	auth := group.Group("/admin/auth")
	auth.POST("/login", middleware.RateLimit(10, time.Minute), h.login)
	auth.PUT("/password", middleware.AdminAuth(signer), h.changePassword)
}

func (h *adminAuthHandler) login(c *gin.Context) {
	var req model.AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：username 和 password 不能为空"))
		return
	}
	resp, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *adminAuthHandler) changePassword(c *gin.Context) {
	var req model.AdminChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：old_password/new_password 必填且新密码至少 8 位"))
		return
	}
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), adminID, req); err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, gin.H{"updated": true})
}
