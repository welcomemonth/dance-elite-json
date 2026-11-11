package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/wxpay"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// ErrPaymentAmountMismatch 表示回调/查单金额与订单金额不一致（T11 金额校验）。
// 路由层据此记录告警并拒绝（returns non-success，微信重推）。
var ErrPaymentAmountMismatch = errors.New("payment amount mismatch")

// PaymentService 是微信支付结果处理服务（开发计划 T11，API 文档 3.2/3.1c）。
//
// 支付成功统一走 settle：单个事务内写 payments 流水（raw_callback 全量留档）
// → 订单 paid + paid_at → 报名单 active。任何一步失败整事务回滚，微信收到
// 非 success 后重推，天然可重试；transaction_id 唯一约束 + 事务内先行查重
// 保证重复回调幂等。
type PaymentService struct {
	stores *store.Store
	notify wxpay.Notifier
	query  wxpay.OrderQuerier
}

// NewPaymentService 构造支付结果服务；notify/query 由真实通道或 mock 通道实现。
func NewPaymentService(stores *store.Store, notify wxpay.Notifier, query wxpay.OrderQuerier) *PaymentService {
	return &PaymentService{stores: stores, notify: notify, query: query}
}

// HandleNotify 处理微信支付结果回调：验签解密 → 金额校验 → 事务结账。
// 返回 nil 表示已确认（路由层按微信协议返回 SUCCESS）；错误时路由层返回
// 非 success，微信会重推。非 SUCCESS 交易状态（关闭/未支付）由 T12 的取消
// 路径处理，这里仅确认收到、不做状态变更。
func (s *PaymentService) HandleNotify(ctx context.Context, headers wxpay.NotifyHeaders, body []byte) error {
	tx, err := s.notify.ParseNotify(headers, body)
	if err != nil {
		return err
	}
	if tx.TradeState != wxpay.TradeStateSuccess {
		return nil
	}
	raw := string(body)
	return s.settle(ctx, tx, &raw)
}

// QueryStatus 查询订单状态（GET /api/v1/orders/{order_id}）。
// 订单仍待支付时主动向微信查单（T11 回调丢失兜底），确认 SUCCESS 则走与
// 回调完全相同的结账事务，避免"钱扣了没报上"。只能查询本人订单。
func (s *PaymentService) QueryStatus(ctx context.Context, userID, orderID int64) (*model.OrderStatusResponse, error) {
	order, err := s.stores.Orders.Get(ctx, orderID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && order.UserID != userID) {
		return nil, httpx.ErrNotFound("订单不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载订单失败，请稍后重试")
	}

	if order.Status == model.OrderStatusPendingPayment {
		queried, qerr := s.query.QueryOrder(ctx, order.OrderNo)
		if qerr != nil {
			return nil, httpx.ErrInternal(qerr, "查询支付状态失败，请稍后重试")
		}
		if queried != nil && queried.OutTradeNo == order.OrderNo &&
			queried.TradeState == wxpay.TradeStateSuccess {
			if err := s.settle(ctx, queried, nil); err != nil {
				return nil, httpx.ErrInternal(err, "确认支付状态失败，请稍后重试")
			}
			order, err = s.stores.Orders.Get(ctx, orderID)
			if err != nil {
				return nil, httpx.ErrInternal(err, "加载订单失败，请稍后重试")
			}
		}
	}
	return orderStatusResponse(order), nil
}

// settle 在单个事务内完成"流水 → 订单 → 报名单"的状态推进。
// rawCallback 为原始回调报文（查单兜底时为 nil，流水保留来源可区分）。
func (s *PaymentService) settle(ctx context.Context, tx *wxpay.Transaction, rawCallback *string) error {
	if tx.TransactionID == "" || tx.OutTradeNo == "" {
		return fmt.Errorf("%w: 交易号或商户订单号缺失", wxpay.ErrInvalidNotify)
	}
	return s.stores.WithTx(ctx, func(sqlTx *sql.Tx) error {
		// 幂等：同一 transaction_id 已入账则直接确认（重复回调/重推）。
		existing, err := s.stores.Payments.GetByTransactionIDTx(ctx, sqlTx, tx.TransactionID)
		if err != nil {
			return err
		}
		if existing != nil {
			return nil
		}

		order, err := s.stores.Orders.GetByOrderNoTx(ctx, sqlTx, tx.OutTradeNo)
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("支付回调订单不存在（out_trade_no=%s）: %w", tx.OutTradeNo, err)
		}
		if err != nil {
			return err
		}
		switch order.Status {
		case model.OrderStatusPendingPayment:
			// 正常路径，继续结账。
		case model.OrderStatusPaid:
			// 已支付但无对应流水（理论上不会发生）：幂等确认，不重复写。
			return nil
		default:
			// canceled/refunding/refunded 却收到支付成功：钱已到账但订单不可用，
			// 必须回滚并告警，人工核查（T15 退款）。
			return fmt.Errorf("订单 %s 状态为 %s，无法结转为已支付", order.OrderNo, order.Status)
		}

		if tx.AmountTotal != order.Amount {
			return fmt.Errorf("%w: 实付 %d 分与订单金额 %d 分不一致（订单 %s）",
				ErrPaymentAmountMismatch, tx.AmountTotal, order.Amount, order.OrderNo)
		}

		notifiedAt := utils.NowISO()
		if _, err := s.stores.Payments.CreateTx(ctx, sqlTx, store.PaymentParams{
			OrderID:       order.ID,
			Type:          model.PaymentTypePay,
			PrepayID:      order.PrepayID,
			TransactionID: &tx.TransactionID,
			Amount:        tx.AmountTotal,
			Status:        model.PaymentStatusSuccess,
			RawCallback:   rawCallback,
			NotifiedAt:    &notifiedAt,
		}); err != nil {
			// 并发回调撞唯一约束：回查确认后按幂等处理。
			if store.IsUniqueViolation(err) {
				if dup, getErr := s.stores.Payments.GetByTransactionIDTx(ctx, sqlTx, tx.TransactionID); getErr == nil && dup != nil {
					return nil
				}
			}
			return err
		}
		if err := s.stores.Orders.MarkPaidTx(ctx, sqlTx, order.ID, paidAtOf(tx)); err != nil {
			return fmt.Errorf("订单 %s 置为已支付失败: %w", order.OrderNo, err)
		}
		if err := s.stores.Registrations.ActivateTx(ctx, sqlTx, order.RegistrationID); err != nil {
			return fmt.Errorf("报名单 %d 置为有效失败: %w", order.RegistrationID, err)
		}
		return nil
	})
}

// paidAtOf 取支付时间：优先微信 success_time（归一化到全库时区格式），
// 缺失或格式异常时退化为处理时刻。
func paidAtOf(tx *wxpay.Transaction) string {
	if tx.SuccessTime != "" {
		if t, err := time.Parse(time.RFC3339, tx.SuccessTime); err == nil {
			return utils.FormatISO(t)
		}
	}
	return utils.NowISO()
}

// orderStatusResponse 组装订单状态响应。
func orderStatusResponse(order *model.Order) *model.OrderStatusResponse {
	return &model.OrderStatusResponse{
		OrderID:        order.ID,
		OrderNo:        order.OrderNo,
		Status:         order.Status,
		Amount:         order.Amount,
		PaidAt:         order.PaidAt,
		RegistrationID: order.RegistrationID,
	}
}
