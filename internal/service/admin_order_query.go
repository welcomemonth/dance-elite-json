package service

import (
	"context"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
)

// AdminOrderQueryService 是后台订单查询服务（开发计划 T17，API 文档 11.1）。
// 只读：列表筛选与详情展示，不改变订单状态。
type AdminOrderQueryService struct {
	stores *store.Store
}

// NewAdminOrderQueryService 构造后台订单查询服务。
func NewAdminOrderQueryService(stores *store.Store) *AdminOrderQueryService {
	return &AdminOrderQueryService{stores: stores}
}

// List 返回后台订单分页列表（赛事/组别/舞种/状态/来源/支付时间/关键字筛选）。
func (s *AdminOrderQueryService) List(ctx context.Context, q model.AdminOrderListQuery) (*model.AdminOrderListResponse, error) {
	if err := validateGroupLevelFilter(q.Level); err != nil {
		return nil, err
	}
	if err := validateDanceTypeFilter(q.DanceType); err != nil {
		return nil, err
	}
	if err := validateAdminOrderStatusFilter(q.Status); err != nil {
		return nil, err
	}
	if err := validateAdminOrderSourceFilter(q.Source); err != nil {
		return nil, err
	}
	paidFrom, paidTo, err := normalizeAdminPaidRange(q.PaidFrom, q.PaidTo)
	if err != nil {
		return nil, err
	}
	page, pageSize := normalizePage(q.Page, q.PageSize)

	rows, total, err := s.stores.Orders.ListAdmin(ctx, store.AdminOrderFilter{
		EventID:         q.EventID,
		Level:           q.Level,
		DanceType:       q.DanceType,
		Status:          q.Status,
		IncludeCanceled: q.IncludeCanceled,
		Source:          q.Source,
		PaidFrom:        paidFrom,
		PaidTo:          paidTo,
		Keyword:         strings.TrimSpace(q.Keyword),
		Page:            page,
		PageSize:        pageSize,
	})
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载订单列表失败，请稍后重试")
	}

	list := make([]model.AdminOrderItem, 0, len(rows))
	for _, r := range rows {
		list = append(list, model.AdminOrderItem{
			OrderNo:       r.OrderNo,
			TransactionID: r.TransactionID,
			EventID:       r.EventID,
			EventName:     r.EventName,
			Level:         r.Level,
			DanceType:     r.DanceType,
			PlayerNo:      r.PlayerNo,
			PlayerName:    r.PlayerName,
			IDType:        r.IDType,
			Amount:        r.Amount,
			Status:        r.Status,
			Source:        r.Source,
			PaidAt:        r.PaidAt,
			CreatedAt:     r.CreatedAt,
		})
	}
	return &model.AdminOrderListResponse{
		List: list, Total: total, Page: page, PageSize: pageSize,
	}, nil
}
