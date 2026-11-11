package router

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/service"
)

// userRateKey 返回按登录用户限流的键（供报名/预下单等接口复用）。
func userRateKey(c *gin.Context) string {
	if id, ok := middleware.GetUserID(c); ok {
		return "user:" + strconv.FormatInt(id, 10)
	}
	return ""
}

// orderHandler 承载小程序订单支付接口（T10，API 文档 3.1b）。
type orderHandler struct {
	svc *service.OrderService
}

// registerOrderRoutes 注册 /api/v1/orders 下的接口。
// 预下单按用户限流 10 次/分钟（支付参数可幂等复用，正常流程只会调一次）。
func registerOrderRoutes(group *gin.RouterGroup, svc *service.OrderService, signer *token.Signer) {
	h := &orderHandler{svc: svc}
	group.POST("/orders/:order_id/prepay",
		middleware.UserAuth(signer),
		middleware.RateLimitKeyed(10, time.Minute, userRateKey),
		h.prepay)
}

// prepay 换取小程序支付参数：参数校验 → 归属/状态校验 → 统一下单与签名（service 层）。
func (h *orderHandler) prepay(c *gin.Context) {
	orderID, err := strconv.ParseInt(c.Param("order_id"), 10, 64)
	if err != nil || orderID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("订单 ID 不合法"))
		return
	}
	userID, ok := middleware.GetUserID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	openid, ok := middleware.GetOpenID(c)
	if !ok || openid == "" {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}

	resp, err := h.svc.Prepay(c.Request.Context(), userID, openid, orderID)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}
