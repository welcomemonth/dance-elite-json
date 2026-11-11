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

// registrationHandler 承载小程序报名下单接口（T9，API 文档 3.1）。
type registrationHandler struct {
	svc *service.RegistrationService
}

// registerRegistrationRoutes 注册 POST /api/v1/events/{event_id}/registrations。
// 需用户登录；按 user_id 限流 1 次/5 秒（API 文档 3.1 注意事项）。
func registerRegistrationRoutes(group *gin.RouterGroup, svc *service.RegistrationService, signer *token.Signer) {
	h := &registrationHandler{svc: svc}
	group.POST("/events/:event_id/registrations",
		middleware.UserAuth(signer),
		middleware.RateLimitKeyed(1, 5*time.Second, func(c *gin.Context) string {
			if id, ok := middleware.GetUserID(c); ok {
				return "user:" + strconv.FormatInt(id, 10)
			}
			return ""
		}),
		h.submit)
}

// submit 提交报名：参数校验 → 校验链 + 事务创建报名单与订单（service 层）。
func (h *registrationHandler) submit(c *gin.Context) {
	eventID, err := strconv.ParseInt(c.Param("event_id"), 10, 64)
	if err != nil || eventID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("赛事 ID 不合法"))
		return
	}
	userID, ok := middleware.GetUserID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}

	var req model.SubmitRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：player_name/id_card_no/dance_type/idempotency_key 为必填项"))
		return
	}

	resp, err := h.svc.Submit(c.Request.Context(), userID, eventID, req)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}
