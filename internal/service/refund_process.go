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
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
	"github.com/meetmonth/dance-elite/internal/pkg/wxpay"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// 退款处理审计动作（admin_logs.action）。
const (
	adminActionRefundApprove = "admin.refund.approve"
	adminActionRefundReject  = "admin.refund.reject"
	adminActionRefundFailed  = "admin.refund.failed"
	adminActionRefundNotify  = "refund.callback"
)

// 退款操作字段上限。
const (
	refundAdjustReasonMaxRunes = 200
	refundRejectReasonMaxRunes = 200
)

// Approve 同意退款（API 文档 11.4.3）：校验金额 → 认领工单 → 调微信退款 API
// → 工单 approved、订单 refunding、写 refund 流水；退款回调完成最终状态。
//
// 微信退款接口失败：工单置 refund_failed、订单回退 paid，返回 41014，可重试。
func (s *RefundService) Approve(ctx context.Context, adminID, ticketID int64, req model.ApproveRefundTicketRequest) (*model.ApproveRefundTicketResponse, error) {
	amount := req.Amount
	adjustReason := strings.TrimSpace(req.AdjustReason)
	switch {
	case amount <= 0:
		return nil, httpx.ErrBadRequest("退款金额必须大于 0")
	case utf8.RuneCountInString(adjustReason) > refundAdjustReasonMaxRunes:
		return nil, httpx.ErrBadRequest(fmt.Sprintf("调整原因不能超过 %d 字", refundAdjustReasonMaxRunes))
	}

	ticket, err := s.stores.RefundTickets.Get(ctx, ticketID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrNotFound("退款工单不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载退款工单失败，请稍后重试")
	}
	if !isRefundApprovable(ticket.Status) {
		return nil, httpx.ErrStateDenied("工单状态不允许同意退款")
	}
	order, err := s.stores.Orders.Get(ctx, ticket.OrderID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrInternal(fmt.Errorf("工单 %d 关联订单缺失", ticketID), "订单数据异常，请联系技术支持")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载订单失败，请稍后重试")
	}
	event, err := s.loadOrderEvent(ctx, order.RegistrationID)
	if err != nil {
		return nil, err
	}

	defaultAmount := refundDefaultAmount(order, event, s.now())
	if amount > order.Amount {
		return nil, httpx.ErrBadRequest("退款金额不能超过实付金额")
	}
	if amount != defaultAmount && adjustReason == "" {
		return nil, httpx.ErrBadRequest("退款金额与退费政策默认金额不一致，请填写调整原因")
	}

	// 认领：processing + 订单 refunding，防并发重复发起。
	if err := s.claimRefund(ctx, adminID, ticketID); err != nil {
		return nil, err
	}

	result, apiErr := s.refunder.Refund(ctx, wxpay.RefundRequest{
		OutTradeNo:  order.OrderNo,
		OutRefundNo: ticket.TicketNo,
		Reason:      truncateRunes(ticket.Reason, 80),
		AmountFen:   amount,
		TotalFen:    order.Amount,
	})
	if apiErr != nil {
		s.recordRefundFailure(ctx, ticketID, &adminID, apiErr)
		return nil, httpx.ErrRefundAPIFailed(apiErr, "微信退款发起失败，工单已标记退款失败，可重试")
	}

	orderStatus, ticketStatus, err := s.finishApprove(ctx, adminID, ticketID, result, amount, adjustReason, defaultAmount)
	if err != nil {
		return nil, err
	}
	return &model.ApproveRefundTicketResponse{
		TicketNo:    ticket.TicketNo,
		Status:      ticketStatus,
		RefundID:    result.RefundID,
		OrderStatus: orderStatus,
	}, nil
}

// claimRefund 在事务内把工单推进到 processing、订单推进到 refunding。
func (s *RefundService) claimRefund(ctx context.Context, adminID, ticketID int64) error {
	return s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		ticket, err := s.stores.RefundTickets.GetTx(ctx, tx, ticketID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("退款工单不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载退款工单失败，请稍后重试")
		}
		if !isRefundApprovable(ticket.Status) {
			return httpx.ErrStateDenied("工单状态不允许同意退款")
		}
		order, err := s.stores.Orders.GetTx(ctx, tx, ticket.OrderID)
		if err != nil {
			return httpx.ErrInternal(err, "加载订单失败，请稍后重试")
		}
		if order.Status != model.OrderStatusPaid && order.Status != model.OrderStatusRefunding {
			return httpx.ErrStateDenied("订单当前状态不允许退款")
		}
		if err := s.stores.RefundTickets.MarkProcessingTx(ctx, tx, ticketID, adminID); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return httpx.ErrStateDenied("工单状态已变化，请刷新后重试")
			}
			return httpx.ErrInternal(err, "更新工单状态失败，请稍后重试")
		}
		if order.Status == model.OrderStatusPaid {
			if err := s.stores.Orders.MarkRefundingTx(ctx, tx, order.ID); err != nil {
				if errors.Is(err, store.ErrConflict) {
					return httpx.ErrStateDenied("订单状态已变化，请刷新后重试")
				}
				return httpx.ErrInternal(err, "更新订单状态失败，请稍后重试")
			}
		}
		return nil
	})
}

// finishApprove 在退款接口受理成功后落库：工单 approved、refund 流水、审计日志。
// 若退款回调抢先完成（工单已 refunded），按幂等成功返回。
func (s *RefundService) finishApprove(ctx context.Context, adminID, ticketID int64,
	result *wxpay.RefundResult, amount int64, adjustReason string, defaultAmount int64) (string, string, error) {
	orderStatus := model.OrderStatusRefunding
	ticketStatus := model.RefundTicketStatusApproved
	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		ticket, err := s.stores.RefundTickets.GetTx(ctx, tx, ticketID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("退款工单不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载退款工单失败，请稍后重试")
		}
		switch ticket.Status {
		case model.RefundTicketStatusRefunded:
			orderStatus = model.OrderStatusRefunded // 回调已先行完成
			ticketStatus = model.RefundTicketStatusRefunded
		case model.RefundTicketStatusApproved:
			// 并发同意已落库，幂等返回。
		case model.RefundTicketStatusProcessing:
			handledAt := utils.NowISO()
			if err := s.stores.RefundTickets.MarkApprovedTx(ctx, tx, ticketID, handledAt); err != nil {
				return httpx.ErrInternal(err, "更新工单状态失败，请稍后重试")
			}
		default:
			return httpx.ErrStateDenied("工单状态已变化，请刷新后重试")
		}

		existing, err := s.stores.Payments.GetByRefundIDTx(ctx, tx, result.RefundID)
		if err != nil {
			return httpx.ErrInternal(err, "查询退款流水失败，请稍后重试")
		}
		if existing == nil {
			if _, err := s.stores.Payments.CreateTx(ctx, tx, store.PaymentParams{
				OrderID:  ticket.OrderID,
				Type:     model.PaymentTypeRefund,
				RefundID: &result.RefundID,
				Amount:   amount,
				Status:   model.PaymentStatusSuccess,
			}); err != nil && !store.IsUniqueViolation(err) {
				return httpx.ErrInternal(err, "写入退款流水失败，请稍后重试")
			}
		}
		if err := adminLog(ctx, s.stores, tx, &adminID, adminActionRefundApprove,
			strPtr("refund_ticket"), &ticketID, map[string]any{
				"ticket_no":      ticket.TicketNo,
				"order_no":       result.OutTradeNo,
				"amount":         amount,
				"default_amount": defaultAmount,
				"adjust_reason":  adjustReason,
				"refund_id":      result.RefundID,
			}); err != nil {
			return httpx.ErrInternal(err, "记录操作日志失败，请稍后重试")
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}
	return orderStatus, ticketStatus, nil
}

// recordRefundFailure 在退款接口失败后回退状态（旁路，失败只记日志不覆盖主错误）。
func (s *RefundService) recordRefundFailure(ctx context.Context, ticketID int64, handlerID *int64, cause error) {
	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		ticket, err := s.stores.RefundTickets.GetTx(ctx, tx, ticketID)
		if err != nil {
			return err
		}
		// 已 approved/refunded 说明退款已受理/完成（如响应超时但微信已受理），
		// 不得降级；以退款回调为准。
		if ticket.Status == model.RefundTicketStatusApproved || ticket.Status == model.RefundTicketStatusRefunded {
			return nil
		}
		if err := s.stores.RefundTickets.MarkRefundFailedTx(ctx, tx, ticketID, handlerID, utils.NowISO()); err != nil &&
			!errors.Is(err, store.ErrConflict) {
			return err
		}
		order, err := s.stores.Orders.GetTx(ctx, tx, ticket.OrderID)
		if err != nil {
			return err
		}
		if order.Status == model.OrderStatusRefunding {
			if err := s.stores.Orders.RevertRefundingTx(ctx, tx, order.ID); err != nil &&
				!errors.Is(err, store.ErrConflict) {
				return err
			}
		}
		return adminLog(ctx, s.stores, tx, handlerID, adminActionRefundFailed,
			strPtr("refund_ticket"), &ticketID, map[string]any{
				"ticket_no": ticket.TicketNo,
				"order_no":  order.OrderNo,
				"error":     truncateRunes(cause.Error(), 300),
			})
	})
	if err != nil {
		logger.FromCtx(ctx).Errorw("退款失败状态回退落库失败，需人工核查",
			"ticket_id", ticketID, "error", err)
	}
}

// Reject 驳回退款（API 文档 11.4.4）：仅待处理工单；订单保持 paid。
func (s *RefundService) Reject(ctx context.Context, adminID, ticketID int64, req model.RejectRefundTicketRequest) (*model.RejectRefundTicketResponse, error) {
	reason := strings.TrimSpace(req.RejectReason)
	switch {
	case reason == "":
		return nil, httpx.ErrBadRequest("驳回原因不能为空")
	case utf8.RuneCountInString(reason) > refundRejectReasonMaxRunes:
		return nil, httpx.ErrBadRequest(fmt.Sprintf("驳回原因不能超过 %d 字", refundRejectReasonMaxRunes))
	}

	var ticketNo string
	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		ticket, err := s.stores.RefundTickets.GetTx(ctx, tx, ticketID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("退款工单不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载退款工单失败，请稍后重试")
		}
		if ticket.Status != model.RefundTicketStatusPending {
			return httpx.ErrStateDenied("仅待处理工单可驳回")
		}
		order, err := s.stores.Orders.GetTx(ctx, tx, ticket.OrderID)
		if err != nil {
			return httpx.ErrInternal(err, "加载订单失败，请稍后重试")
		}
		if order.Status != model.OrderStatusPaid {
			return httpx.ErrStateDenied("订单当前状态不允许驳回退款")
		}
		handledAt := utils.NowISO()
		if err := s.stores.RefundTickets.MarkRejectedTx(ctx, tx, ticketID, adminID, reason, handledAt); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return httpx.ErrStateDenied("工单状态已变化，请刷新后重试")
			}
			return httpx.ErrInternal(err, "更新工单状态失败，请稍后重试")
		}
		if err := adminLog(ctx, s.stores, tx, &adminID, adminActionRefundReject,
			strPtr("refund_ticket"), &ticketID, map[string]any{
				"ticket_no":     ticket.TicketNo,
				"order_no":      order.OrderNo,
				"reject_reason": reason,
			}); err != nil {
			return httpx.ErrInternal(err, "记录操作日志失败，请稍后重试")
		}
		ticketNo = ticket.TicketNo
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &model.RejectRefundTicketResponse{
		TicketNo:    ticketNo,
		Status:      model.RefundTicketStatusRejected,
		OrderStatus: model.OrderStatusPaid,
	}, nil
}

// refundDefaultAmount 按退费政策计算默认退款金额：抽签前全额，抽签后个人原因 50%。
// 伤病全额等特殊情况由管理员调整金额并填写调整原因。
func refundDefaultAmount(order *model.Order, event *model.Event, now time.Time) int64 {
	if event.LotteryAt == nil || *event.LotteryAt == "" {
		return order.Amount
	}
	lotteryAt, err := utils.ParseISO(*event.LotteryAt)
	if err != nil {
		return order.Amount
	}
	if !now.Before(lotteryAt) {
		return order.Amount / 2
	}
	return order.Amount
}

// isRefundApprovable 判断工单是否可发起/重试同意退款。
func isRefundApprovable(status string) bool {
	switch status {
	case model.RefundTicketStatusPending,
		model.RefundTicketStatusProcessing,
		model.RefundTicketStatusRefundFailed:
		return true
	default:
		return false
	}
}

// truncateRunes 按字符截断字符串（不切断 UTF-8）。
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
