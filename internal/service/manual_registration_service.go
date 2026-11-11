package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// 人工报名审计动作（admin_logs.action）。
const adminActionManualRegistration = "admin.manual_registration.create"

// ManualRegistrationService 是后台人工报名服务（开发计划 T16，API 文档 11.5）：
// 无大陆身份证号选手由管理员手工建档（证件号哈希 + 人工出生日期，同一套年龄规则
// 校验手选组别），并在同一事务内创建已支付订单（线下收款/免费）与支付流水。
//
// 安全约定：证件号明文只在请求参数与本函数栈内；审计日志只记录哈希与 player_no。
type ManualRegistrationService struct {
	stores  *store.Store
	players *PlayerService
	now     func() time.Time
}

// NewManualRegistrationService 构造人工报名服务。
func NewManualRegistrationService(stores *store.Store, players *PlayerService) *ManualRegistrationService {
	return &ManualRegistrationService{stores: stores, players: players, now: time.Now}
}

// Create 创建人工报名（API 文档 11.5）：校验赛事/组别/年龄 → 事务内建档、判重、
// 抢名额、建报名单（active, source=manual）、订单（paid）与支付流水（manual 备注）。
func (s *ManualRegistrationService) Create(ctx context.Context, adminID int64, req model.ManualRegistrationRequest) (*model.ManualRegistrationResponse, error) {
	in, err := normalizeManualRegistration(req)
	if err != nil {
		return nil, err
	}

	event, err := s.stores.Events.Get(ctx, in.EventID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrNotFound("赛事不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
	}
	if !isPublishedVisible(event) || deriveMiniappStatus(event, s.now()) != model.MiniappStatusRegistering {
		return nil, httpx.ErrNotInRegPeriod("赛事不在可报名状态")
	}

	prepared, err := s.players.PrepareIdentity(IdentityInput{
		IDType:    in.IDType,
		IDNumber:  in.IDNumber,
		Name:      in.PlayerName,
		BirthDate: in.BirthDate,
		Gender:    in.Gender,
	})
	if err != nil {
		return nil, err
	}
	birth, err := utils.ParseDate(*prepared.BirthDate)
	if err != nil {
		return nil, httpx.ErrInternal(err, "解析出生日期失败")
	}
	birthYear := birth.Year()

	var resp *model.ManualRegistrationResponse
	err = s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		ev, err := s.stores.Events.GetForRegistrationTx(ctx, tx, in.EventID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("赛事不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
		}
		if !isPublishedVisible(ev) || deriveMiniappStatus(ev, s.now()) != model.MiniappStatusRegistering {
			return httpx.ErrNotInRegPeriod("赛事不在可报名状态")
		}

		group, err := s.stores.Groups.GetTx(ctx, tx, in.GroupID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("赛事组别不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载赛事组别失败，请稍后重试")
		}
		if group.EventID != in.EventID {
			return httpx.ErrBadRequest("组别不属于该赛事")
		}
		if !group.Enabled {
			return httpx.ErrGroupDisabled(fmt.Sprintf("该组别（%s %s）已关闭报名", group.Level, danceTypeLabel(group.DanceType)))
		}
		if birthYear < group.BirthYearFrom || birthYear > group.BirthYearTo {
			return httpx.ErrAgeNotAllowed(fmt.Sprintf(
				"不符合参赛年龄：选手出生于 %d 年，%s 组别要求出生年份 %d-%d 年",
				birthYear, group.Level, group.BirthYearFrom, group.BirthYearTo))
		}

		if in.CSTicketID > 0 {
			ticket, err := s.stores.CSTickets.GetTx(ctx, tx, in.CSTicketID)
			if errors.Is(err, store.ErrNotFound) {
				return httpx.ErrNotFound("客服工单不存在")
			}
			if err != nil {
				return httpx.ErrInternal(err, "加载客服工单失败，请稍后重试")
			}
			if ticket.RelatedRegistrationID != nil {
				return httpx.ErrConflict("客服工单已关联报名")
			}
		}

		player, _, err := s.stores.Players.MatchOrCreate(ctx, tx, store.PlayerParams{
			Name:        prepared.Name,
			IDType:      prepared.IDType,
			IDCardHash:  prepared.IDCardHash,
			Gender:      prepared.Gender,
			BirthDate:   prepared.BirthDate,
			Nationality: in.Nationality,
		})
		if err != nil {
			return httpx.ErrInternal(err, "创建选手档案失败，请稍后重试")
		}

		inFlights, err := s.stores.Registrations.InFlightByPlayerEventTx(ctx, tx, player.ID, in.EventID)
		if err != nil {
			return httpx.ErrInternal(err, "查询报名记录失败，请稍后重试")
		}
		for _, r := range inFlights {
			if r.Level != group.Level {
				return httpx.ErrLevelAlreadyTaken(fmt.Sprintf(
					"该选手已报名该赛事的 %s 组别，同一赛事一人只能报名一个组别", r.Level))
			}
			if r.GroupID == group.ID {
				return httpx.ErrDanceTypeDuplicate("该选手已报名该组别，请勿重复提交")
			}
		}

		took, snap, err := s.stores.Groups.TakeSlotTx(ctx, tx, group.ID)
		if err != nil {
			return httpx.ErrInternal(err, "占用名额失败，请稍后重试")
		}
		if !took {
			if !snap.Enabled {
				return httpx.ErrGroupDisabled("该组别已关闭报名")
			}
			return httpx.ErrQuotaFull("该组别名额已满",
				model.QuotaFullData{GroupID: snap.ID, Level: snap.Level, DanceType: snap.DanceType})
		}

		manualUser, err := s.stores.Users.GetOrCreateManualUserTx(ctx, tx)
		if err != nil {
			return httpx.ErrInternal(err, "初始化人工报名账号失败，请稍后重试")
		}
		var csTicketID *int64
		if in.CSTicketID > 0 {
			csTicketID = &in.CSTicketID
		}
		regID, err := s.stores.Registrations.CreateTx(ctx, tx, store.RegistrationParams{
			EventID:      in.EventID,
			GroupID:      group.ID,
			PlayerID:     player.ID,
			UserID:       manualUser.ID,
			DanceType:    group.DanceType,
			Institution:  in.Institution,
			Status:       model.RegistrationStatusActive,
			Source:       model.OrderSourceManual,
			CSTicketID:   csTicketID,
			ContactPhone: &in.ContactPhone,
			SpecialReq:   in.Remark,
		})
		if store.IsUniqueViolation(err) {
			return httpx.ErrDanceTypeDuplicate("该选手已报名该组别，请勿重复提交")
		}
		if err != nil {
			return httpx.ErrInternal(err, "创建报名单失败，请稍后重试")
		}
		if csTicketID != nil {
			if err := s.stores.CSTickets.LinkRegistrationTx(ctx, tx, *csTicketID, regID); err != nil {
				if errors.Is(err, store.ErrConflict) {
					return httpx.ErrConflict("客服工单已关联报名")
				}
				return httpx.ErrInternal(err, "关联客服工单失败，请稍后重试")
			}
		}

		orderID, orderNo, err := s.createManualOrder(ctx, tx, manualUser.ID, regID, in.Amount)
		if err != nil {
			return err
		}
		note := fmt.Sprintf("后台人工录入（操作人 admin:%d，收费方式 %s）", adminID, in.ChargeType)
		notifiedAt := utils.FormatISO(s.now())
		if _, err := s.stores.Payments.CreateTx(ctx, tx, store.PaymentParams{
			OrderID:     orderID,
			Type:        model.PaymentTypePay,
			Amount:      in.Amount,
			Status:      model.PaymentStatusSuccess,
			RawCallback: &note,
			NotifiedAt:  &notifiedAt,
		}); err != nil {
			return httpx.ErrInternal(err, "写入支付流水失败，请稍后重试")
		}

		if err := adminLog(ctx, s.stores, tx, &adminID, adminActionManualRegistration,
			strPtr("registration"), &regID, map[string]any{
				"player_no":    player.PlayerNo,
				"id_card_hash": prepared.IDCardHash,
				"birth_date":   *prepared.BirthDate,
				"event_id":     in.EventID,
				"group_id":     group.ID,
				"cs_ticket_id": in.CSTicketID,
				"amount":       in.Amount,
				"charge_type":  in.ChargeType,
			}); err != nil {
			return httpx.ErrInternal(err, "记录操作日志失败，请稍后重试")
		}
		resp = &model.ManualRegistrationResponse{
			RegistrationID: regID,
			OrderNo:        orderNo,
			PlayerNo:       player.PlayerNo,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// createManualOrder 在事务内生成 order_no 并创建已支付订单（source=manual）；
// order_no 撞 UNIQUE 时换号重试。
func (s *ManualRegistrationService) createManualOrder(ctx context.Context, tx *sql.Tx, userID, registrationID, amount int64) (int64, string, error) {
	var lastErr error
	for range maxOrderNoRetries {
		orderNo, err := generateOrderNo(s.now())
		if err != nil {
			return 0, "", httpx.ErrInternal(err, "生成订单号失败，请稍后重试")
		}
		orderID, err := s.stores.Orders.CreateTx(ctx, tx, store.OrderParams{
			OrderNo:        orderNo,
			RegistrationID: registrationID,
			UserID:         userID,
			Amount:         amount,
			Status:         model.OrderStatusPaid,
			Source:         model.OrderSourceManual,
			PaidAt:         strPtr(utils.FormatISO(s.now())),
		})
		if err == nil {
			return orderID, orderNo, nil
		}
		lastErr = err
		if !store.IsUniqueViolation(err) {
			return 0, "", httpx.ErrInternal(err, "创建订单失败，请稍后重试")
		}
	}
	return 0, "", httpx.ErrInternal(lastErr, "创建订单失败，请稍后重试")
}
