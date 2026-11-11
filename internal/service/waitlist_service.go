package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// WaitlistService 是候补队列服务（开发计划 T14，API 文档 5.1–5.3 / 12.1–12.2）。
//
// 用户侧：名额满时提交候补（不收费、不预占名额）、查看本人候补、排队中自助取消。
// 后台侧：按赛事/组别查询队列、标记已通知、手动关联已支付订单（converted）、移出队列。
//
// 并发约定：候补提交在 BEGIN IMMEDIATE 事务内复核名额并取 MAX(position_no)+1，
// 依赖 SQLite 单写入者串行化；同报名单元同证件号的 waiting 重复由部分唯一索引兜底。
type WaitlistService struct {
	stores  *store.Store
	players *PlayerService
	now     func() time.Time
}

// NewWaitlistService 构造候补队列服务；players 用于可选的身份证校验与哈希。
func NewWaitlistService(stores *store.Store, players *PlayerService) *WaitlistService {
	return &WaitlistService{stores: stores, players: players, now: time.Now}
}

// Submit 提交候补（API 文档 5.1）。
//
// 校验链：表单（40001/41001）→ 赛事报名中（40401/41006）→ 组别属于该赛事（40401）
// → 年龄匹配（41002）→ 组别启用（41007）→ 名额已满（40001，未满引导直接报名）
// → [事务] 复核 → 同证件号已排队（41010）→ 取队列序号写入。
func (s *WaitlistService) Submit(ctx context.Context, userID, eventID int64, req model.SubmitWaitlistRequest) (*model.SubmitWaitlistResponse, error) {
	prepared, err := s.prepareWaitlistInput(req)
	if err != nil {
		return nil, err
	}

	event, err := s.stores.Events.Get(ctx, eventID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !isPublishedVisible(event)) {
		return nil, httpx.ErrNotFound("赛事不存在或未发布")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
	}
	if deriveMiniappStatus(event, s.now()) != model.MiniappStatusRegistering {
		return nil, httpx.ErrNotInRegPeriod("该赛事当前不在报名期，无法提交候补")
	}

	group, err := s.stores.Groups.Get(ctx, req.GroupID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrNotFound("赛事组别不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载赛事组别失败，请稍后重试")
	}
	if err := checkWaitlistGroup(eventID, group, prepared.BirthYear); err != nil {
		return nil, err
	}

	var resp *model.SubmitWaitlistResponse
	err = s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		// 事务内复核赛事报名期与组别名额：防"校验后、写单前"被关闭报名或释放名额。
		ev, err := s.stores.Events.GetForRegistrationTx(ctx, tx, eventID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("赛事不存在或未发布")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
		}
		if !isPublishedVisible(ev) {
			return httpx.ErrNotFound("赛事不存在或未发布")
		}
		if deriveMiniappStatus(ev, s.now()) != model.MiniappStatusRegistering {
			return httpx.ErrNotInRegPeriod("该赛事当前不在报名期，无法提交候补")
		}
		g, err := s.stores.Groups.GetTx(ctx, tx, req.GroupID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("赛事组别不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载赛事组别失败，请稍后重试")
		}
		if err := checkWaitlistGroup(eventID, g, prepared.BirthYear); err != nil {
			return err
		}

		hash := ""
		if prepared.IDCardHash != nil {
			hash = *prepared.IDCardHash
		}
		exists, err := s.stores.Waitlists.WaitingExistsTx(ctx, tx, eventID, req.GroupID, hash)
		if err != nil {
			return httpx.ErrInternal(err, "查询候补队列失败，请稍后重试")
		}
		if exists {
			return httpx.ErrWaitlistDuplicate("该证件号已在候补队列中，请勿重复提交")
		}

		position, err := s.stores.Waitlists.NextPositionTx(ctx, tx, eventID, req.GroupID)
		if err != nil {
			return httpx.ErrInternal(err, "获取候补序号失败，请稍后重试")
		}
		id, err := s.stores.Waitlists.InsertTx(ctx, tx, store.WaitlistParams{
			EventID:      eventID,
			GroupID:      req.GroupID,
			UserID:       userID,
			DanceType:    g.DanceType,
			Name:         prepared.Name,
			IDCardHash:   prepared.IDCardHash,
			IDCardMasked: prepared.IDCardMasked,
			Institution:  prepared.Institution,
			Phone:        prepared.Phone,
			PositionNo:   position,
		})
		if store.IsUniqueViolation(err) {
			return httpx.ErrWaitlistDuplicate("该证件号已在候补队列中，请勿重复提交")
		}
		if err != nil {
			return httpx.ErrInternal(err, "提交候补失败，请稍后重试")
		}
		resp = &model.SubmitWaitlistResponse{
			WaitlistID: id,
			PositionNo: position,
			Status:     model.WaitlistStatusWaiting,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// ListMine 返回当前用户的候补记录（API 文档 5.2），含已放弃/已移除等留档记录。
func (s *WaitlistService) ListMine(ctx context.Context, userID int64, page, pageSize int) (*model.MyWaitlistListResponse, error) {
	page, pageSize = normalizePage(page, pageSize)
	rows, total, err := s.stores.Waitlists.ListByUser(ctx, userID, page, pageSize)
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载候补记录失败，请稍后重试")
	}
	items := make([]model.MyWaitlistItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, model.MyWaitlistItem{
			WaitlistID: r.WaitlistID,
			EventID:    r.EventID,
			EventName:  r.EventName,
			Level:      r.Level,
			DanceType:  r.DanceType,
			PositionNo: r.PositionNo,
			Status:     r.Status,
			CreatedAt:  r.CreatedAt,
		})
	}
	return &model.MyWaitlistListResponse{List: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// Cancel 用户自助取消候补（API 文档 5.3）：排队中 → given_up，仅移出队列，不涉及退款。
// 已通知/已转报名返回 41011（需联系客服）；重复取消幂等返回成功。
func (s *WaitlistService) Cancel(ctx context.Context, userID, waitlistID int64) error {
	handledAt := utils.FormatISO(s.now())
	return s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		w, err := s.stores.Waitlists.GetTx(ctx, tx, waitlistID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("候补记录不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载候补记录失败，请稍后重试")
		}
		if w.UserID == nil || *w.UserID != userID {
			// 不区分"不存在"与"非本人"，防枚举。
			return httpx.ErrNotFound("候补记录不存在")
		}
		switch w.Status {
		case model.WaitlistStatusGivenUp:
			return nil // 重复取消幂等
		case model.WaitlistStatusWaiting:
			if err := s.stores.Waitlists.CancelOwnTx(ctx, tx, waitlistID, userID, handledAt); err != nil {
				if errors.Is(err, store.ErrConflict) {
					// 并发下状态已变化：回读后按最新状态给出结果。
					latest, getErr := s.stores.Waitlists.GetTx(ctx, tx, waitlistID)
					if getErr == nil && latest.UserID != nil && *latest.UserID == userID {
						if latest.Status == model.WaitlistStatusGivenUp {
							return nil // 另一个并发请求已完成取消，幂等成功
						}
						return httpx.ErrWaitlistNotCancelable("候补状态已变化，无法自助取消；如需帮助请联系客服")
					}
					return httpx.ErrNotFound("候补记录不存在")
				}
				return httpx.ErrInternal(err, "取消候补失败，请稍后重试")
			}
			return nil
		default:
			return httpx.ErrWaitlistNotCancelable("候补已通知或已结束，如需取消请联系客服")
		}
	})
}

// normalizePage 规范化分页参数（默认 1 / 20，上限 100）。
func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
