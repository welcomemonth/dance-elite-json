package router

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/pkg/wxpay"
	"github.com/meetmonth/dance-elite/internal/service"
)

// wechatNotifyBodyLimit 是回调报文大小上限（正常报文 < 10KB，防滥用）。
const wechatNotifyBodyLimit = 64 << 10

// paymentHandler 承载支付回调与订单状态查询（T11，API 文档 3.2/3.1c）。
type paymentHandler struct {
	svc *service.PaymentService
}

// registerPaymentRoutes 注册支付相关接口：
//   - POST /api/v1/payments/wechat/notify：微信平台回调，无登录态，按验签鉴权；
//   - GET  /api/v1/orders/{order_id}：本人订单状态查询（含主动查单兜底）。
func registerPaymentRoutes(group *gin.RouterGroup, svc *service.PaymentService, signer *token.Signer) {
	h := &paymentHandler{svc: svc}
	group.POST("/payments/wechat/notify", h.wechatNotify)
	group.GET("/orders/:order_id",
		middleware.UserAuth(signer),
		middleware.RateLimitKeyed(30, time.Minute, userRateKey),
		h.orderStatus)
}

// registerRefundNotifyRoute 注册微信退款结果回调（T15）：公共接口，按验签鉴权。
func registerRefundNotifyRoute(group *gin.RouterGroup, svc *service.RefundService) {
	h := &refundNotifyHandler{svc: svc}
	group.POST("/payments/wechat/refund-notify", h.wechatRefundNotify)
}

// refundNotifyHandler 处理微信退款结果回调。
type refundNotifyHandler struct {
	svc *service.RefundService
}

// wechatRefundNotify 响应遵循微信支付协议：成功 200 + {"code":"SUCCESS"}；
// 验签失败 401；处理失败 500 + 非 success，微信会按策略重推。
func (h *refundNotifyHandler) wechatRefundNotify(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, wechatNotifyBodyLimit))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "回调报文读取失败"})
		return
	}
	headers := wxpay.NotifyHeaders{
		Timestamp: c.GetHeader("Wechatpay-Timestamp"),
		Nonce:     c.GetHeader("Wechatpay-Nonce"),
		Signature: c.GetHeader("Wechatpay-Signature"),
		Serial:    c.GetHeader("Wechatpay-Serial"),
	}
	if err := h.svc.HandleRefundNotify(c.Request.Context(), headers, body); err != nil {
		l := logger.FromCtx(c.Request.Context())
		if errors.Is(err, wxpay.ErrInvalidNotify) {
			l.Warnw("微信退款回调验签失败", "err", err)
			c.JSON(http.StatusUnauthorized, gin.H{"code": "FAIL", "message": "签名验证失败"})
			return
		}
		l.Errorw("微信退款回调处理失败", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": "FAIL", "message": "处理失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "成功"})
}

// wechatNotify 处理微信支付结果回调。
// 响应遵循微信支付协议（非统一包装）：成功 200 + {"code":"SUCCESS"}；
// 验签失败 401；处理失败 500 + 非 success，微信会按策略重推。
func (h *paymentHandler) wechatNotify(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, wechatNotifyBodyLimit))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "回调报文读取失败"})
		return
	}
	headers := wxpay.NotifyHeaders{
		Timestamp: c.GetHeader("Wechatpay-Timestamp"),
		Nonce:     c.GetHeader("Wechatpay-Nonce"),
		Signature: c.GetHeader("Wechatpay-Signature"),
		Serial:    c.GetHeader("Wechatpay-Serial"),
	}

	if err := h.svc.HandleNotify(c.Request.Context(), headers, body); err != nil {
		l := logger.FromCtx(c.Request.Context())
		if errors.Is(err, wxpay.ErrInvalidNotify) {
			l.Warnw("微信支付回调验签失败", "err", err)
			c.JSON(http.StatusUnauthorized, gin.H{"code": "FAIL", "message": "签名验证失败"})
			return
		}
		if errors.Is(err, service.ErrPaymentAmountMismatch) {
			// 金额不一致属于高风险事件：拒绝并告警，人工核查。
			l.Errorw("微信支付回调金额不一致，已拒绝", "err", err)
		} else {
			l.Errorw("微信支付回调处理失败", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "FAIL", "message": "处理失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "成功"})
}

// orderStatus 查询订单状态；待支付时 service 会向微信主动查单（T11 兜底）。
func (h *paymentHandler) orderStatus(c *gin.Context) {
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
	resp, err := h.svc.QueryStatus(c.Request.Context(), userID, orderID)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}
