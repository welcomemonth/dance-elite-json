package service

import (
	"context"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
)

// AdminRegistrationQueryService 是后台报名名单查询服务（开发计划 T17）。
// 只读：按赛事/组别/舞种/资料完成度筛选，并标注每行缺失资料项。
type AdminRegistrationQueryService struct {
	stores *store.Store
}

// NewAdminRegistrationQueryService 构造后台报名名单查询服务。
func NewAdminRegistrationQueryService(stores *store.Store) *AdminRegistrationQueryService {
	return &AdminRegistrationQueryService{stores: stores}
}

// List 返回后台报名名单分页列表（status 缺省 active）。
func (s *AdminRegistrationQueryService) List(ctx context.Context, q model.AdminRegistrationListQuery) (*model.AdminRegistrationListResponse, error) {
	if err := validateGroupLevelFilter(q.Level); err != nil {
		return nil, err
	}
	if err := validateDanceTypeFilter(q.DanceType); err != nil {
		return nil, err
	}
	if err := validateAdminRegistrationStatusFilter(q.Status); err != nil {
		return nil, err
	}
	if err := validateMaterialStateFilter(q.MaterialState); err != nil {
		return nil, err
	}
	if err := validateMissingItemFilter(q.MissingItem); err != nil {
		return nil, err
	}
	page, pageSize := normalizePage(q.Page, q.PageSize)

	rows, total, err := s.stores.Registrations.ListAdmin(ctx, store.AdminRegistrationFilter{
		EventID:       q.EventID,
		GroupID:       q.GroupID,
		Level:         q.Level,
		DanceType:     q.DanceType,
		Status:        q.Status,
		MaterialState: q.MaterialState,
		MissingItem:   q.MissingItem,
		Keyword:       strings.TrimSpace(q.Keyword),
		Page:          page,
		PageSize:      pageSize,
	})
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载报名名单失败，请稍后重试")
	}

	list := make([]model.AdminRegistrationItem, 0, len(rows))
	for _, r := range rows {
		status := MaterialStatusOf(&r.Registration)
		list = append(list, model.AdminRegistrationItem{
			RegistrationID:    r.Registration.ID,
			OrderNo:           r.OrderNo,
			EventID:           r.Registration.EventID,
			EventName:         r.EventName,
			GroupID:           r.Registration.GroupID,
			Level:             r.Level,
			DanceType:         r.Registration.DanceType,
			PlayerNo:          r.PlayerNo,
			PlayerName:        r.PlayerName,
			IDType:            r.IDType,
			Institution:       r.Registration.Institution,
			ContactPhone:      r.ContactPhone,
			Status:            r.Registration.Status,
			Source:            r.Registration.Source,
			MaterialCompleted: status.Completed,
			MaterialTotal:     status.Total,
			MissingItems:      status.Missing,
			MaterialUpdatedAt: r.Registration.MaterialUpdatedAt,
			CreatedAt:         r.Registration.CreatedAt,
		})
	}
	return &model.AdminRegistrationListResponse{
		List: list, Total: total, Page: page, PageSize: pageSize,
	}, nil
}
