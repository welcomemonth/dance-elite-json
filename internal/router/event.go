package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/service"
)

// publicEventHandler 承载小程序端赛事只读接口（T7，API 文档 2.1/2.2）。
type publicEventHandler struct {
	svc *service.PublicEventService
}

// registerEventQueryRoutes 注册赛事查询路由（group 需已挂 OptionalUserAuth）。
func registerEventQueryRoutes(group *gin.RouterGroup, svc *service.PublicEventService) {
	r := &publicEventHandler{svc: svc}
	group.GET("/events", r.list)
	group.GET("/events/:id", r.detail)
}

// list 赛事列表（API 文档 2.1）。
func (h *publicEventHandler) list(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	var uid *int64
	if userID > 0 {
		uid = &userID
	}
	resp, err := h.svc.List(c.Request.Context(), model.MiniappEventListQuery{
		Status:   c.Query("status"),
		Page:     queryInt(c, "page", 1),
		PageSize: queryIntMax(c, "page_size", 20, 100),
	}, uid)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// detail 赛事详情（API 文档 2.2）。
func (h *publicEventHandler) detail(c *gin.Context) {
	id, ok := eventIDParam(c)
	if !ok {
		return
	}
	// userID 暂不参与详情计算，但保持可选登录解析的一致性（角标在列表已给出）。
	resp, err := h.svc.Detail(c.Request.Context(), id, nil)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}
