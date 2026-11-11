package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/service"
)

// adminRegistrationHandler 承载后台报名名单接口（开发计划 T17）。
type adminRegistrationHandler struct {
	svc *service.AdminRegistrationQueryService
}

// registerAdminRegistrationRoutes 注册 /api/v1/admin/registrations 下的接口。
// 列表对 admin/super_admin/cs 全部只读。
func registerAdminRegistrationRoutes(group *gin.RouterGroup, svc *service.AdminRegistrationQueryService) {
	h := &adminRegistrationHandler{svc: svc}
	group.Group("/registrations").GET("", h.list)
}

// list 报名名单分页列表（赛事/组别/舞种/状态/资料完成度筛选）。
func (h *adminRegistrationHandler) list(c *gin.Context) {
	eventID, ok := optionalInt64Query(c, "event_id")
	if !ok {
		httpx.AbortError(c, httpx.ErrBadRequest("event_id 不合法"))
		return
	}
	groupID, ok := optionalInt64Query(c, "group_id")
	if !ok {
		httpx.AbortError(c, httpx.ErrBadRequest("group_id 不合法"))
		return
	}
	resp, err := h.svc.List(c.Request.Context(), model.AdminRegistrationListQuery{
		EventID:       eventID,
		GroupID:       groupID,
		Level:         c.Query("level"),
		DanceType:     c.Query("dance_type"),
		Status:        c.Query("status"),
		MaterialState: c.Query("material_state"),
		MissingItem:   c.Query("missing_item"),
		Keyword:       c.Query("keyword"),
		Page:          queryInt(c, "page", 1),
		PageSize:      queryIntMax(c, "page_size", 20, 100),
	})
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}
