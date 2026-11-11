package router

import (
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
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

// orderHandler 承载小程序订单支付、取消与历史查询接口
// （T10/T12，API 文档 3.1b/3.5/3.6）。
type orderHandler struct {
	svc *service.OrderService
}

// registerOrderRoutes 注册 /api/v1/orders 下的接口。
// 预下单按用户限流 10 次/分钟（支付参数可幂等复用，正常流程只会调一次）；
// 取消与历史列表按用户限流 30 次/分钟（取消幂等，重复调用安全）。
func registerOrderRoutes(group *gin.RouterGroup, svc *service.OrderService, signer *token.Signer) {
	h := &orderHandler{svc: svc}
	group.POST("/orders/:order_id/prepay",
		middleware.UserAuth(signer),
		middleware.RateLimitKeyed(10, time.Minute, userRateKey),
		h.prepay)
	group.POST("/orders/:order_id/cancel",
		middleware.UserAuth(signer),
		middleware.RateLimitKeyed(30, time.Minute, userRateKey),
		h.cancel)
	group.GET("/orders",
		middleware.UserAuth(signer),
		middleware.RateLimitKeyed(30, time.Minute, userRateKey),
		h.listHistory)
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

// cancel 用户主动取消待支付订单（T12，POST /orders/{order_id}/cancel）。
// 请求体可选：{"reason":"user_cancel|pay_failed"}，缺省 user_cancel。
func (h *orderHandler) cancel(c *gin.Context) {
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

	var req model.CancelOrderRequest
	// 允许空请求体（小程序关闭收银台时可直接 POST 取消）。
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：reason 取值不合法"))
		return
	}

	resp, err := h.svc.Cancel(c.Request.Context(), userID, orderID, req.Reason)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// listHistory 我的历史订单（T12，GET /orders）：含已取消记录，统一分页。
func (h *orderHandler) listHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	resp, err := h.svc.ListHistory(c.Request.Context(), userID,
		queryInt(c, "page", 1), queryIntMax(c, "page_size", 20, 100))
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}
