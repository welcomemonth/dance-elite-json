package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// AdminList 返回后台候补队列（API 文档 12.1）：按赛事/组别筛选，组内按队列序号排序。
// released_quota 为筛选范围内报名单元的可用名额合计，供队列顶部"已释放 N 个名额"提示。
func (s *WaitlistService) AdminList(ctx context.Context, q model.AdminWaitlistListQuery) (*model.AdminWaitlistListResponse, error) {
	if err := validateWaitlistStatusFilter(q.Status); err != nil {
		return nil, err
	}
	if q.Level != "" && q.Level != model.GroupLevelU11 && q.Level != model.GroupLevelU13 && q.Level != model.GroupLevelU15 {
		return nil, httpx.ErrBadRequest("level 取值不合法")
	}
	if q.DanceType != "" && q.DanceType != model.DanceTypeClassical && q.DanceType != model.DanceTypeFolk {
		return nil, httpx.ErrBadRequest("dance_type 取值不合法")
	}

	page, pageSize := normalizePage(q.Page, q.PageSize)
	rows, total, err := s.stores.Waitlists.ListAdmin(ctx, store.AdminWaitlistFilter{
		EventID:   q.EventID,
		Level:     q.Level,
		DanceType: q.DanceType,
		Status:    q.Status,
		Page:      page,
		PageSize:  pageSize,
	})
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载候补队列失败，请稍后重试")
	}
	released, err := s.stores.Waitlists.ReleasedQuota(ctx, q.EventID, q.Level, q.DanceType)
	if err != nil {
		return nil, httpx.ErrInternal(err, "统计释放名额失败，请稍后重试")
	}

	items := make([]model.AdminWaitlistItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, model.AdminWaitlistItem{
			WaitlistID:   r.WaitlistID,
			PositionNo:   r.PositionNo,
			Name:         r.Name,
			IDCardMasked: r.IDCardMasked,
			Institution:  r.Institution,
			Phone:        r.Phone,
			Status:       r.Status,
			EventID:      r.EventID,
			EventName:    r.EventName,
			GroupID:      r.GroupID,
			Level:        r.Level,
			DanceType:    r.DanceType,
			CreatedAt:    r.CreatedAt,
			NotifiedAt:   r.NotifiedAt,
			HandledAt:    r.HandledAt,
		})
	}
	return &model.AdminWaitlistListResponse{
		List:          items,
		Total:         total,
		Page:          page,
		PageSize:      pageSize,
		ReleasedQuota: released,
	}, nil
}

// AdminNotify 标记已通知（API 文档 12.2）：waiting/notified → notified，刷新 notified_at，
// 通知结果备注写入 admin_logs（表结构不设备注列，操作留痕统一走审计日志）。
func (s *WaitlistService) AdminNotify(ctx context.Context, adminID, waitlistID int64, note string) (*model.AdminWaitlistActionResponse, error) {
	note = strings.TrimSpace(note)
	if err := validateWaitlistNote(note); err != nil {
		return nil, err
	}
	notifiedAt := utils.FormatISO(s.now())
	var resp *model.AdminWaitlistActionResponse
	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		w, err := s.stores.Waitlists.GetTx(ctx, tx, waitlistID)
		if err != nil {
			return mapWaitlistLoadErr(err)
		}
		if !isWaitlistActive(w.Status) {
			return waitlistStateDenied(w.Status)
		}
		if err := s.stores.Waitlists.NotifyTx(ctx, tx, waitlistID, notifiedAt); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return waitlistStateDenied(w.Status)
			}
			return httpx.ErrInternal(err, "标记已通知失败，请稍后重试")
		}
		detail := map[string]any{"note": note, "event_id": w.EventID, "group_id": w.GroupID}
		if err := adminLog(ctx, s.stores, tx, &adminID, "admin.waitlist.notify", strPtr("waitlist"), &waitlistID, detail); err != nil {
			return httpx.ErrInternal(err, "记录操作日志失败，请稍后重试")
		}
		resp = &model.AdminWaitlistActionResponse{WaitlistID: waitlistID, Status: model.WaitlistStatusNotified}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// AdminRemove 移出队列（API 文档 12.2）：仅改状态不删除，写移出原因与 handled_at。
func (s *WaitlistService) AdminRemove(ctx context.Context, adminID, waitlistID int64, reason string) (*model.AdminWaitlistActionResponse, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, httpx.ErrBadRequest("移出原因不能为空")
	}
	if utf8.RuneCountInString(reason) > 200 {
		return nil, httpx.ErrBadRequest("移出原因不能超过 200 字")
	}
	handledAt := utils.FormatISO(s.now())
	var resp *model.AdminWaitlistActionResponse
	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		w, err := s.stores.Waitlists.GetTx(ctx, tx, waitlistID)
		if err != nil {
			return mapWaitlistLoadErr(err)
		}
		if !isWaitlistActive(w.Status) {
			return waitlistStateDenied(w.Status)
		}
		if err := s.stores.Waitlists.FinishTx(ctx, tx, waitlistID, model.WaitlistStatusRemoved, handledAt); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return waitlistStateDenied(w.Status)
			}
			return httpx.ErrInternal(err, "移出候补队列失败，请稍后重试")
		}
		detail := map[string]any{"reason": reason, "event_id": w.EventID, "group_id": w.GroupID}
		if err := adminLog(ctx, s.stores, tx, &adminID, "admin.waitlist.remove", strPtr("waitlist"), &waitlistID, detail); err != nil {
			return httpx.ErrInternal(err, "记录操作日志失败，请稍后重试")
		}
		resp = &model.AdminWaitlistActionResponse{WaitlistID: waitlistID, Status: model.WaitlistStatusRemoved}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// AdminConvert 手动把候补记录与已支付订单关联（API 文档 12.2）：waiting/notified → converted。
// 订单必须属于候补提交人、状态为已支付，且其报名单与候补为同一赛事同一报名单元。
func (s *WaitlistService) AdminConvert(ctx context.Context, adminID, waitlistID int64, orderNo string) (*model.AdminWaitlistActionResponse, error) {
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return nil, httpx.ErrBadRequest("订单号不能为空")
	}
	handledAt := utils.FormatISO(s.now())
	var resp *model.AdminWaitlistActionResponse
	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		w, err := s.stores.Waitlists.GetTx(ctx, tx, waitlistID)
		if err != nil {
			return mapWaitlistLoadErr(err)
		}
		if w.Status == model.WaitlistStatusConverted {
			// 重复确认转报名幂等返回。
			resp = &model.AdminWaitlistActionResponse{WaitlistID: waitlistID, Status: model.WaitlistStatusConverted}
			return nil
		}
		if !isWaitlistActive(w.Status) {
			return waitlistStateDenied(w.Status)
		}

		order, err := s.stores.Orders.GetByOrderNoTx(ctx, tx, orderNo)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrBadRequest("订单不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载订单失败，请稍后重试")
		}
		if w.UserID != nil && order.UserID != *w.UserID {
			return httpx.ErrBadRequest("订单不属于该候补提交人")
		}
		if order.Status != model.OrderStatusPaid {
			return httpx.ErrBadRequest("订单未支付，不能关联为已转报名")
		}
		reg, err := s.stores.Registrations.GetTx(ctx, tx, order.RegistrationID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrBadRequest("订单关联的报名单不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载报名单失败，请稍后重试")
		}
		if reg.EventID != w.EventID || reg.GroupID != w.GroupID {
			return httpx.ErrBadRequest("订单对应的赛事/报名单元与候补记录不一致")
		}

		if err := s.stores.Waitlists.FinishTx(ctx, tx, waitlistID, model.WaitlistStatusConverted, handledAt); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return waitlistStateDenied(w.Status)
			}
			return httpx.ErrInternal(err, "确认转报名失败，请稍后重试")
		}
		detail := map[string]any{"order_no": orderNo, "event_id": w.EventID, "group_id": w.GroupID}
		if err := adminLog(ctx, s.stores, tx, &adminID, "admin.waitlist.convert", strPtr("waitlist"), &waitlistID, detail); err != nil {
			return httpx.ErrInternal(err, "记录操作日志失败，请稍后重试")
		}
		resp = &model.AdminWaitlistActionResponse{WaitlistID: waitlistID, Status: model.WaitlistStatusConverted}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// mapWaitlistLoadErr 把加载候补记录的错误映射为业务错误。
func mapWaitlistLoadErr(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return httpx.ErrNotFound("候补记录不存在")
	}
	return httpx.ErrInternal(err, "加载候补记录失败，请稍后重试")
}

// waitlistStateDenied 构造 41013（候补状态不允许该操作）。
func waitlistStateDenied(status string) error {
	return httpx.ErrWaitlistStateDenied(fmt.Sprintf("候补记录当前状态为 %s，不允许该操作", status))
}
