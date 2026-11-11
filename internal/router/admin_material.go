package router

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/service"
)

// adminMaterialHandler 承载后台资料催收接口（开发计划 T17，API 文档 12.3–12.4）。
type adminMaterialHandler struct {
	svc *service.MaterialCollectionService
}

// registerAdminMaterialRoutes 注册 /api/v1/admin/materials 下的接口：
// 名单与标记已联系 cs/admin 均可用。
func registerAdminMaterialRoutes(group *gin.RouterGroup, svc *service.MaterialCollectionService) {
	h := &adminMaterialHandler{svc: svc}
	materials := group.Group("/materials")
	materials.GET("/missing", h.list)
	materials.POST("/missing/:registration_id/contacted", h.contacted)
}

// list 资料未传齐名单（赛事/组别/舞种/缺失项/截止状态筛选）。
func (h *adminMaterialHandler) list(c *gin.Context) {
	eventID, ok := optionalInt64Query(c, "event_id")
	if !ok {
		httpx.AbortError(c, httpx.ErrBadRequest("event_id 不合法"))
		return
	}
	resp, err := h.svc.List(c.Request.Context(), model.AdminMissingMaterialQuery{
		EventID:       eventID,
		Level:         c.Query("level"),
		DanceType:     c.Query("dance_type"),
		MissingItem:   c.Query("missing_item"),
		DeadlineState: c.Query("deadline_state"),
		Page:          queryInt(c, "page", 1),
		PageSize:      queryIntMax(c, "page_size", 20, 100),
	})
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// contacted 标记已联系（追加催收记录，截止后只读）。
func (h *adminMaterialHandler) contacted(c *gin.Context) {
	adminID, ok := middleware.GetAdminID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}
	registrationID, err := strconv.ParseInt(c.Param("registration_id"), 10, 64)
	if err != nil || registrationID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("报名单 ID 不合法"))
		return
	}
	var req model.MarkMaterialContactedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请求参数错误：note 不能超过 200 字"))
		return
	}
	resp, err := h.svc.MarkContacted(c.Request.Context(), adminID, registrationID, req.Note)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}
