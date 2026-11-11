package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// 资料催收审计动作（admin_logs.action）。
const adminActionMaterialContacted = "admin.material.contacted"

// materialContactNoteMaxRunes 是催收备注长度上限。
const materialContactNoteMaxRunes = 200

// MaterialCollectionService 是资料催收服务（开发计划 T17，API 文档 12.3–12.4）：
// 未传齐名单查询（photo/audio/program_name 任一为空）与标记已联系留痕。
type MaterialCollectionService struct {
	stores *store.Store
	now    func() time.Time
}

// NewMaterialCollectionService 构造资料催收服务。
func NewMaterialCollectionService(stores *store.Store) *MaterialCollectionService {
	return &MaterialCollectionService{stores: stores, now: time.Now}
}

// List 返回资料未传齐名单（active 报名），逐行标注缺失项与是否已联系。
func (s *MaterialCollectionService) List(ctx context.Context, q model.AdminMissingMaterialQuery) (*model.AdminMissingMaterialListResponse, error) {
	if err := validateGroupLevelFilter(q.Level); err != nil {
		return nil, err
	}
	if err := validateDanceTypeFilter(q.DanceType); err != nil {
		return nil, err
	}
	if err := validateMissingItemFilter(q.MissingItem); err != nil {
		return nil, err
	}
	if err := validateDeadlineStateFilter(q.DeadlineState); err != nil {
		return nil, err
	}
	page, pageSize := normalizePage(q.Page, q.PageSize)

	rows, total, err := s.stores.Registrations.ListMissing(ctx, store.AdminMissingMaterialFilter{
		EventID:       q.EventID,
		Level:         q.Level,
		DanceType:     q.DanceType,
		MissingItem:   q.MissingItem,
		DeadlineState: q.DeadlineState,
		Now:           utils.FormatISO(s.now()),
		Page:          page,
		PageSize:      pageSize,
	})
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载资料催收名单失败，请稍后重试")
	}

	list := make([]model.AdminMissingMaterialItem, 0, len(rows))
	for _, r := range rows {
		status := MaterialStatusOf(&r.Registration)
		list = append(list, model.AdminMissingMaterialItem{
			RegistrationID:    r.Registration.ID,
			OrderNo:           r.OrderNo,
			EventID:           r.Registration.EventID,
			EventName:         r.EventName,
			GroupID:           r.Registration.GroupID,
			Level:             r.Level,
			DanceType:         r.Registration.DanceType,
			PlayerNo:          r.PlayerNo,
			PlayerName:        r.PlayerName,
			ContactPhone:      r.ContactPhone,
			MaterialCompleted: status.Completed,
			MaterialTotal:     status.Total,
			MissingItems:      status.Missing,
			MaterialDeadline:  r.MaterialCheckAt,
			MaterialUpdatedAt: r.Registration.MaterialUpdatedAt,
			Contacted:         r.ContactedAt != nil,
			ContactedAt:       r.ContactedAt,
		})
	}
	return &model.AdminMissingMaterialListResponse{
		List: list, Total: total, Page: page, PageSize: pageSize,
	}, nil
}

// MarkContacted 标记已联系（API 文档 12.4）：仅 active 报名且未过资料截止时间；
// 每次调用追加一条催收记录（时间 + 备注）并写审计日志。
func (s *MaterialCollectionService) MarkContacted(ctx context.Context, adminID, registrationID int64, note string) (*model.MarkMaterialContactedResponse, error) {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > materialContactNoteMaxRunes {
		return nil, httpx.ErrBadRequest(fmt.Sprintf("催收备注不能超过 %d 字", materialContactNoteMaxRunes))
	}

	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		reg, err := s.stores.Registrations.GetTx(ctx, tx, registrationID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("报名单不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载报名单失败，请稍后重试")
		}
		if reg.Status != model.RegistrationStatusActive {
			return httpx.ErrMaterialNotAllowed("仅有效报名可标记资料催收")
		}
		event, err := s.stores.Events.GetForRegistrationTx(ctx, tx, reg.EventID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("赛事不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
		}
		if event.MaterialCheckAt != nil && *event.MaterialCheckAt != "" &&
			utils.FormatISO(s.now()) >= *event.MaterialCheckAt {
			return httpx.ErrMaterialDeadline("资料上传已截止，名单已锁定")
		}

		var notePtr *string
		if note != "" {
			notePtr = &note
		}
		if err := s.stores.MaterialContacts.CreateTx(ctx, tx, registrationID, adminID, notePtr); err != nil {
			return httpx.ErrInternal(err, "记录催收信息失败，请稍后重试")
		}
		if err := adminLog(ctx, s.stores, tx, &adminID, adminActionMaterialContacted,
			strPtr("registration"), &registrationID, map[string]any{
				"event_id": reg.EventID,
				"note":     note,
			}); err != nil {
			return httpx.ErrInternal(err, "记录操作日志失败，请稍后重试")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &model.MarkMaterialContactedResponse{RegistrationID: registrationID, Contacted: true}, nil
}
