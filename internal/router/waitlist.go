package router

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/service"
)

// waitlistHandler 承载小程序候补队列接口（开发计划 T14，API 文档 5.1–5.3）。
type waitlistHandler struct {
	svc *service.WaitlistService
}

// registerWaitlistRoutes 注册小程序端候补接口，全部需要用户登录。
func registerWaitlistRoutes(group *gin.RouterGroup, svc *service.WaitlistService, signer *token.Signer) {
	h := &waitlistHandler{svc: svc}
	auth := middleware.UserAuth(signer)
	group.POST("/events/:event_id/waitlists",
		auth, middleware.RateLimitKeyed(10, time.Minute, userRateKey), h.submit)
	group.GET("/waitlists",
		auth, middleware.RateLimitKeyed(30, time.Minute, userRateKey), h.listMine)
	group.DELETE("/waitlists/:waitlist_id",
		auth, middleware.RateLimitKeyed(30, time.Minute, userRateKey), h.cancel)
}

// submit 提交候补：名额已满时登记姓名/证件号（可空）/机构/电话入队。
func (h *waitlistHandler) submit(c *gin.Context) {
	eventID, err := strconv.ParseInt(c.Param("event_id"), 10, 64)
	if err != nil || eventID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("赛事 ID 不合法"))
		return
	}
	var req model.SubmitWaitlistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：group_id/name/phone 为必填项"))
		return
	}
	userID, ok := middleware.GetUserID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	resp, err := h.svc.Submit(c.Request.Context(), userID, eventID, req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// listMine 返回当前用户的候补记录（统一分页）。
func (h *waitlistHandler) listMine(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	resp, err := h.svc.ListMine(c.Request.Context(), userID,
		queryInt(c, "page", 1), queryIntMax(c, "page_size", 20, 100))
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// cancel 用户自助取消候补（仅排队中可取消）。
func (h *waitlistHandler) cancel(c *gin.Context) {
	waitlistID, err := strconv.ParseInt(c.Param("waitlist_id"), 10, 64)
	if err != nil || waitlistID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("候补 ID 不合法"))
		return
	}
	userID, ok := middleware.GetUserID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	if err := h.svc.Cancel(c.Request.Context(), userID, waitlistID); err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, nil)
}
