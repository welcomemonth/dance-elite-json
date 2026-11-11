package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/wxpay"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// 退款域审计动作（admin_logs.action）。
const (
	adminActionRefundCreate = "admin.refund.create"
)

// 退款工单字段上限（用户可见文本，按 rune 计）。
const (
	refundReasonMaxRunes    = 500
	refundApplicantMaxRunes = 50
)

// RefundService 是退款工单服务（开发计划 T15，API 文档 11.4）。
//
// 流程：客服/管理员后台代录工单（pending）→ 管理员同意（调微信退款 API）
// 或驳回；退款结果以微信退款回调为准（订单 refunded / 报名 withdrawn / 名额释放）。
type RefundService struct {
	stores   *store.Store
	refunder wxpay.Refunder
	notifier wxpay.RefundNotifier
	now      func() time.Time
}

// NewRefundService 构造退款工单服务；refunder/notifier 由真实通道或 mock 实现
// （组合根传入同一支付网关）。
func NewRefundService(stores *store.Store, refunder wxpay.Refunder, notifier wxpay.RefundNotifier) *RefundService {
	return &RefundService{stores: stores, refunder: refunder, notifier: notifier, now: time.Now}
}

// CreateTicket 后台代录退款工单（T15）。
//
// 校验链：参数（40001）→ 订单存在（40401）→ 订单已支付且未过报名截止
// （41015）→ 同订单无进行中工单（40901）→ 写入 pending 工单并落审计日志。
func (s *RefundService) CreateTicket(ctx context.Context, adminID int64, req model.CreateRefundTicketRequest) (*model.CreateRefundTicketResponse, error) {
	orderNo := strings.TrimSpace(req.OrderNo)
	reason := strings.TrimSpace(req.Reason)
	applicant := strings.TrimSpace(req.Applicant)
	switch {
	case orderNo == "":
		return nil, httpx.ErrBadRequest("订单号不能为空")
	case reason == "":
		return nil, httpx.ErrBadRequest("退款诉求不能为空")
	case utf8.RuneCountInString(reason) > refundReasonMaxRunes:
		return nil, httpx.ErrBadRequest(fmt.Sprintf("退款诉求不能超过 %d 字", refundReasonMaxRunes))
	case utf8.RuneCountInString(applicant) > refundApplicantMaxRunes:
		return nil, httpx.ErrBadRequest(fmt.Sprintf("登记来源不能超过 %d 字", refundApplicantMaxRunes))
	}
	if applicant == "" {
		applicant = "客服代录"
	}

	var resp *model.CreateRefundTicketResponse
	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		order, err := s.stores.Orders.GetByOrderNoTx(ctx, tx, orderNo)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("订单不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载订单失败，请稍后重试")
		}
		event, err := s.loadOrderEventTx(ctx, tx, order.RegistrationID)
		if err != nil {
			return err
		}
		if err := checkRefundEligibility(order, event, s.now()); err != nil {
			return asRefundRuleError(err)
		}

		active, err := s.stores.RefundTickets.HasActiveByOrderTx(ctx, tx, order.ID)
		if err != nil {
			return httpx.ErrInternal(err, "查询退款工单失败，请稍后重试")
		}
		if active {
			return httpx.ErrConflict("该订单已有进行中的退款工单，请勿重复登记")
		}

		ticketNo, err := generateRefundTicketNo(s.now())
		if err != nil {
			return httpx.ErrInternal(err, "生成工单号失败，请稍后重试")
		}
		ticketID, err := s.stores.RefundTickets.CreateTx(ctx, tx, store.RefundTicketParams{
			TicketNo:  ticketNo,
			OrderID:   order.ID,
			Reason:    reason,
			Applicant: &applicant,
			Status:    model.RefundTicketStatusPending,
			CreatedBy: &adminID,
		})
		if err != nil {
			return httpx.ErrInternal(err, "创建退款工单失败，请稍后重试")
		}
		if err := adminLog(ctx, s.stores, tx, &adminID, adminActionRefundCreate,
			strPtr("refund_ticket"), &ticketID, map[string]any{
				"order_no":  order.OrderNo,
				"reason":    reason,
				"applicant": applicant,
				"amount":    order.Amount,
			}); err != nil {
			return httpx.ErrInternal(err, "记录操作日志失败，请稍后重试")
		}
		resp = &model.CreateRefundTicketResponse{
			TicketID: ticketID,
			TicketNo: ticketNo,
			Status:   model.RefundTicketStatusPending,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// loadOrderEvent 经订单的报名单加载赛事（非事务路径，同意退款前计算退费政策）。
func (s *RefundService) loadOrderEvent(ctx context.Context, registrationID int64) (*model.Event, error) {
	registration, err := s.stores.Registrations.Get(ctx, registrationID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrInternal(
			fmt.Errorf("订单关联的报名单 %d 不存在", registrationID), "订单数据异常，请联系技术支持")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载报名信息失败，请稍后重试")
	}
	event, err := s.stores.Events.Get(ctx, registration.EventID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrInternal(
			fmt.Errorf("报名单 %d 关联的赛事 %d 不存在", registrationID, registration.EventID),
			"订单数据异常，请联系技术支持")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
	}
	return event, nil
}

// loadOrderEventTx 经订单的报名单加载赛事（退款规则时间线依据）。
func (s *RefundService) loadOrderEventTx(ctx context.Context, tx *sql.Tx, registrationID int64) (*model.Event, error) {
	registration, err := s.stores.Registrations.GetTx(ctx, tx, registrationID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrInternal(
			fmt.Errorf("订单关联的报名单 %d 不存在", registrationID), "订单数据异常，请联系技术支持")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载报名信息失败，请稍后重试")
	}
	event, err := s.stores.Events.GetForRegistrationTx(ctx, tx, registration.EventID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrInternal(
			fmt.Errorf("报名单 %d 关联的赛事 %d 不存在", registrationID, registration.EventID),
			"订单数据异常，请联系技术支持")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
	}
	return event, nil
}

// asRefundRuleError 把退款规则内部错误包装为 50001，业务错误原样返回。
func asRefundRuleError(err error) error {
	var biz *httpx.BizError
	if errors.As(err, &biz) {
		return biz
	}
	return httpx.ErrInternal(err, "核验退款规则失败，请稍后重试")
}

// generateRefundTicketNo 生成不可推测的工单号：TK + 14 位紧凑时间 + 6 位随机数字
// （如 TK20270101103000123456）。唯一性由 ticket_no UNIQUE 兜底。
func generateRefundTicketNo(now time.Time) (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	digits := make([]byte, len(buf))
	for i, b := range buf {
		digits[i] = '0' + b%10
	}
	return "TK" + now.In(utils.ChinaLoc).Format("20060102150405") + string(digits), nil
}
