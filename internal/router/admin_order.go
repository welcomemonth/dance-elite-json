package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/service"
)

// adminOrderHandler 承载后台订单查询接口（开发计划 T17，API 文档 11.1）。
type adminOrderHandler struct {
	svc *service.AdminOrderQueryService
}

// registerAdminOrderRoutes 注册 /api/v1/admin/orders 下的接口。
// 列表对 admin/super_admin/cs 全部只读。
func registerAdminOrderRoutes(group *gin.RouterGroup, svc *service.AdminOrderQueryService) {
	h := &adminOrderHandler{svc: svc}
	group.Group("/orders").GET("", h.list)
}

// list 订单分页列表（赛事/组别/舞种/状态/来源/支付时间/关键字筛选）。
func (h *adminOrderHandler) list(c *gin.Context) {
	eventID, ok := optionalInt64Query(c, "event_id")
	if !ok {
		httpx.AbortError(c, httpx.ErrBadRequest("event_id 不合法"))
		return
	}
	resp, err := h.svc.List(c.Request.Context(), model.AdminOrderListQuery{
		EventID:         eventID,
		Level:           c.Query("level"),
		DanceType:       c.Query("dance_type"),
		Status:          c.Query("status"),
		IncludeCanceled: queryBool(c, "include_canceled"),
		Source:          c.Query("source"),
		PaidFrom:        c.Query("paid_from"),
		PaidTo:          c.Query("paid_to"),
		Keyword:         c.Query("keyword"),
		Page:            queryInt(c, "page", 1),
		PageSize:        queryIntMax(c, "page_size", 20, 100),
	})
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// queryBool 解析布尔 query 参数（true/1 视为真，缺省为假）。
func queryBool(c *gin.Context, name string) bool {
	switch c.Query(name) {
	case "true", "1":
		return true
	default:
		return false
	}
}
