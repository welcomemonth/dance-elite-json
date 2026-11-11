package router

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/service"
)

// adminRegistrationHandler 承载后台报名名单接口（开发计划 T17）。
type adminRegistrationHandler struct {
	svc *service.AdminRegistrationQueryService
}

// registerAdminRegistrationRoutes 注册 /api/v1/admin/registrations 下的接口与
// 赛事名单导出接口（API 文档 12.5）。
// 列表对 admin/super_admin/cs 全部只读；导出仅 admin / super_admin。
func registerAdminRegistrationRoutes(group *gin.RouterGroup, svc *service.AdminRegistrationQueryService) {
	h := &adminRegistrationHandler{svc: svc}
	group.Group("/registrations").GET("", h.list)
	export := group.Group("", middleware.RequireRole(model.RoleSuperAdmin, model.RoleAdmin))
	export.GET("/events/:id/registrations/export", h.export)
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

// exportMIME 是 Excel（xlsx）的 Content-Type。
const exportMIME = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// export 按赛事导出报名名单 Excel（仅 admin / super_admin）。
func (h *adminRegistrationHandler) export(c *gin.Context) {
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	eventID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || eventID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("赛事 ID 非法"))
		return
	}
	file, err := h.svc.Export(c.Request.Context(), adminID, eventID, queryBool(c, "include_canceled"))
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	disposition := fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		file.Name, url.PathEscape(file.Name))
	c.Header("Content-Disposition", disposition)
	c.Data(http.StatusOK, exportMIME, file.Content)
}
