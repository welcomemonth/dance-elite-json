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

// authHandler 承载小程序认证相关接口（T4）。
type authHandler struct {
	svc *service.AuthService
}

// registerAuthRoutes 注册 /api/v1/auth 下的接口。
//
// 登录限流 10 次/分钟/IP（API 文档 1.1）；手机号绑定需要用户 token。
func registerAuthRoutes(group *gin.RouterGroup, svc *service.AuthService, signer *token.Signer) {
	h := &authHandler{svc: svc}
	auth := group.Group("/auth")
	auth.POST("/wx-login", middleware.RateLimit(10, time.Minute), h.wxLogin)
	auth.PUT("/phone", middleware.UserAuth(signer), h.bindPhone)
}

func (h *authHandler) wxLogin(c *gin.Context) {
	var req model.WxLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：code 不能为空"))
		return
	}
	resp, err := h.svc.WxLogin(c.Request.Context(), req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *authHandler) bindPhone(c *gin.Context) {
	var req model.BindPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：phone_code 不能为空"))
		return
	}
	userID, ok := middleware.GetUserID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	resp, err := h.svc.BindPhone(c.Request.Context(), userID, req.PhoneCode)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}
