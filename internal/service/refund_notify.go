package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
	"github.com/meetmonth/dance-elite/internal/pkg/wxpay"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// HandleRefundNotify 处理微信退款结果回调（T15）：验签解密 → 按 refund_id 幂等
// → 订单 refunded、报名 withdrawn、名额释放、工单 refunded。
//
// 返回 nil 表示已确认（路由层按微信协议返回 SUCCESS）；错误时返回非 success，
// 微信会重推。PROCESSING 状态不改变本地状态。
func (s *RefundService) HandleRefundNotify(ctx context.Context, headers wxpay.NotifyHeaders, body []byte) error {
	result, err := s.notifier.ParseRefundNotify(headers, body)
	if err != nil {
		return err
	}
	switch result.RefundStatus {
	case wxpay.RefundStatusSuccess:
		return s.settleRefund(ctx, result, string(body))
	case wxpay.RefundStatusProcessing:
		return nil
	case wxpay.RefundStatusClosed, wxpay.RefundStatusAbnormal:
		return s.settleRefundFailure(ctx, result)
	default:
		logger.FromCtx(ctx).Warnw("未知的微信退款状态，已忽略",
			"refund_id", result.RefundID, "status", result.RefundStatus)
		return nil
	}
}

// settleRefund 在单个事务内完成退款成功后的状态推进。
func (s *RefundService) settleRefund(ctx context.Context, result *wxpay.RefundNotifyResult, raw string) error {
	if result.RefundID == "" || result.OutRefundNo == "" {
		return fmt.Errorf("退款回调缺少 refund_id/out_refund_no")
	}
	return s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		ticket, err := s.stores.RefundTickets.GetByTicketNoTx(ctx, tx, result.OutRefundNo)
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("退款回调对应的工单不存在（out_refund_no=%s）", result.OutRefundNo)
		}
		if err != nil {
			return err
		}
		notifiedAt := utils.NowISO()
		if ticket.Status == model.RefundTicketStatusRefunded {
			// 幂等：重复回调只补齐流水回调信息。
			return s.completeRefundPaymentTx(ctx, tx, ticket.OrderID, result, raw, notifiedAt)
		}
		if !isRefundSettleable(ticket.Status) {
			return fmt.Errorf("工单 %s 状态为 %s，无法完成退款", ticket.TicketNo, ticket.Status)
		}

		order, err := s.stores.Orders.GetTx(ctx, tx, ticket.OrderID)
		if err != nil {
			return fmt.Errorf("退款回调加载订单 %d 失败: %w", ticket.OrderID, err)
		}
		if order.Status != model.OrderStatusPaid && order.Status != model.OrderStatusRefunding &&
			order.Status != model.OrderStatusRefunded {
			return fmt.Errorf("订单 %s 状态为 %s，无法完成退款", order.OrderNo, order.Status)
		}

		if err := s.stores.RefundTickets.MarkRefundedTx(ctx, tx, ticket.ID, notifiedAt); err != nil {
			return fmt.Errorf("工单 %s 置为已退款失败: %w", ticket.TicketNo, err)
		}
		if order.Status != model.OrderStatusRefunded {
			if err := s.stores.Orders.MarkRefundedTx(ctx, tx, order.ID); err != nil {
				return fmt.Errorf("订单 %s 置为已退款失败: %w", order.OrderNo, err)
			}
		}
		registration, err := s.stores.Registrations.GetTx(ctx, tx, order.RegistrationID)
		if err != nil {
			return fmt.Errorf("退款回调加载报名单 %d 失败: %w", order.RegistrationID, err)
		}
		switch registration.Status {
		case model.RegistrationStatusActive:
			if err := s.stores.Registrations.WithdrawTx(ctx, tx, registration.ID); err != nil {
				return fmt.Errorf("报名单 %d 置为退赛失败: %w", registration.ID, err)
			}
			if err := s.stores.Groups.ReleaseSlotTx(ctx, tx, registration.GroupID); err != nil {
				return fmt.Errorf("释放组别 %d 名额失败: %w", registration.GroupID, err)
			}
		case model.RegistrationStatusWithdrawn:
			// 已退赛（重复回调/数据已推进）：不重复释放名额。
		default:
			return fmt.Errorf("报名单 %d 状态为 %s，无法完成退款", registration.ID, registration.Status)
		}

		if err := s.completeRefundPaymentTx(ctx, tx, order.ID, result, raw, notifiedAt); err != nil {
			return err
		}
		return adminLog(ctx, s.stores, tx, nil, adminActionRefundNotify,
			strPtr("refund_ticket"), &ticket.ID, map[string]any{
				"ticket_no":    ticket.TicketNo,
				"order_no":     order.OrderNo,
				"refund_id":    result.RefundID,
				"amount":       result.RefundAmount,
				"order_status": model.OrderStatusRefunded,
			})
	})
}

// completeRefundPaymentTx 写入/补齐退款流水（按 refund_id 幂等）。
func (s *RefundService) completeRefundPaymentTx(ctx context.Context, tx *sql.Tx, orderID int64,
	result *wxpay.RefundNotifyResult, raw, notifiedAt string) error {
	existing, err := s.stores.Payments.GetByRefundIDTx(ctx, tx, result.RefundID)
	if err != nil {
		return err
	}
	if existing != nil {
		return s.stores.Payments.CompleteRefundTx(ctx, tx, existing.ID, result.RefundAmount, raw, notifiedAt)
	}
	_, err = s.stores.Payments.CreateTx(ctx, tx, store.PaymentParams{
		OrderID:     orderID,
		Type:        model.PaymentTypeRefund,
		RefundID:    &result.RefundID,
		Amount:      result.RefundAmount,
		Status:      model.PaymentStatusSuccess,
		RawCallback: &raw,
		NotifiedAt:  &notifiedAt,
	})
	return err
}

// settleRefundFailure 处理退款关闭/异常回调：工单 refund_failed、订单回退 paid，
// 流水标记失败，供管理员重试或人工核查。
func (s *RefundService) settleRefundFailure(ctx context.Context, result *wxpay.RefundNotifyResult) error {
	return s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		ticket, err := s.stores.RefundTickets.GetByTicketNoTx(ctx, tx, result.OutRefundNo)
		if errors.Is(err, store.ErrNotFound) {
			logger.FromCtx(ctx).Warnw("退款失败回调对应的工单不存在，已忽略",
				"out_refund_no", result.OutRefundNo)
			return nil
		}
		if err != nil {
			return err
		}
		if ticket.Status == model.RefundTicketStatusRefunded {
			return nil // 成功回调已处理
		}
		if err := s.stores.RefundTickets.MarkRefundFailedTx(ctx, tx, ticket.ID, nil, utils.NowISO()); err != nil &&
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
		if result.RefundID != "" {
			payment, err := s.stores.Payments.GetByRefundIDTx(ctx, tx, result.RefundID)
			if err != nil {
				return err
			}
			if payment != nil {
				if err := s.stores.Payments.MarkRefundFailedTx(ctx, tx, payment.ID); err != nil &&
					!errors.Is(err, store.ErrNotFound) {
					return err
				}
			}
		}
		return adminLog(ctx, s.stores, tx, nil, adminActionRefundFailed,
			strPtr("refund_ticket"), &ticket.ID, map[string]any{
				"ticket_no": ticket.TicketNo,
				"order_no":  order.OrderNo,
				"refund_id": result.RefundID,
				"status":    result.RefundStatus,
			})
	})
}

// isRefundSettleable 判断工单是否可被成功回调完成。
func isRefundSettleable(status string) bool {
	switch status {
	case model.RefundTicketStatusApproved,
		model.RefundTicketStatusProcessing,
		model.RefundTicketStatusRefundFailed:
		return true
	default:
		return false
	}
}
