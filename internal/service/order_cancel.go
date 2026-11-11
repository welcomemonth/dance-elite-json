package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// errOrderNotCancelable 表示订单不在可取消状态（已支付/退款中/已退款）。
// 用户主动取消映射为 40901；支付失败回调路径视为无需处理（不覆盖资金事实）。
var errOrderNotCancelable = errors.New("order is not cancelable")

// orderCanceler 承载订单取消事务（开发计划 T12）。
//
// 取消 = 单个事务内：orders → canceled + 取消原因/时间 → registrations → canceled
// → event_groups.taken - 1（名额即时释放）→ 写 payments(type=pay, status=closed) 留档。
// 重复取消是幂等的：订单已 canceled 时直接返回当前留档，不重复释放名额或写流水。
//
// 该类型同时供 OrderService（用户主动取消）与 PaymentService（微信回调/查单确认
// 支付失败或关闭）使用，避免 service 之间互相依赖。
type orderCanceler struct {
	stores *store.Store
}

// newOrderCanceler 构造订单取消器。
func newOrderCanceler(stores *store.Store) *orderCanceler {
	return &orderCanceler{stores: stores}
}

// cancelPending 在事务内取消待支付订单并释放名额。
// 订单已取消时幂等返回；订单已支付/退款中返回 errOrderNotCancelable；
// 报名单或名额计数不一致属数据异常，整事务回滚。
func (c *orderCanceler) cancelPending(ctx context.Context, orderID int64, reason string) (*model.Order, error) {
	var out *model.Order
	err := c.stores.WithTx(ctx, func(tx *sql.Tx) error {
		order, err := c.stores.Orders.GetTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		// 幂等：重复取消不产生任何副作用（不重复递减 taken、不重复写流水）。
		if order.Status == model.OrderStatusCanceled {
			out = order
			return nil
		}
		if order.Status != model.OrderStatusPendingPayment {
			return fmt.Errorf("%w: 订单 %s 当前状态为 %s", errOrderNotCancelable, order.OrderNo, order.Status)
		}

		registration, err := c.stores.Registrations.GetTx(ctx, tx, order.RegistrationID)
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("取消订单 %s 失败：报名单 %d 不存在", order.OrderNo, order.RegistrationID)
		}
		if err != nil {
			return fmt.Errorf("取消订单 %s 失败：加载报名单: %w", order.OrderNo, err)
		}
		if registration.Status != model.RegistrationStatusPendingPayment {
			return fmt.Errorf("取消订单 %s 失败：报名单 %d 状态为 %s", order.OrderNo, registration.ID, registration.Status)
		}

		canceledAt := utils.NowISO()
		if err := c.stores.Orders.CancelTx(ctx, tx, order.ID, reason, canceledAt); err != nil {
			return fmt.Errorf("取消订单 %s 失败: %w", order.OrderNo, err)
		}
		if err := c.stores.Registrations.CancelPendingTx(ctx, tx, registration.ID); err != nil {
			return fmt.Errorf("取消报名单 %d 失败: %w", registration.ID, err)
		}
		if err := c.stores.Groups.ReleaseSlotTx(ctx, tx, registration.GroupID); err != nil {
			return fmt.Errorf("释放组别 %d 名额失败: %w", registration.GroupID, err)
		}
		// 支付流水留档：取消的支付尝试也全量保留，供后台对账（T17）。
		if _, err := c.stores.Payments.CreateTx(ctx, tx, store.PaymentParams{
			OrderID:  order.ID,
			Type:     model.PaymentTypePay,
			PrepayID: order.PrepayID,
			Amount:   order.Amount,
			Status:   model.PaymentStatusClosed,
		}); err != nil {
			return fmt.Errorf("写订单 %s 关闭流水失败: %w", order.OrderNo, err)
		}

		order.Status = model.OrderStatusCanceled
		order.CancelReason = &reason
		order.CanceledAt = &canceledAt
		out = order
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// cancelByOrderNo 按商户订单号取消待支付订单（微信回调/查单路径）。
// 订单不存在或已结束（已支付/退款）视为无需处理：失败通知不应覆盖资金事实，
// 也不应让微信无限重推。
func (c *orderCanceler) cancelByOrderNo(ctx context.Context, orderNo, reason string) error {
	order, err := c.stores.Orders.GetByOrderNo(ctx, orderNo)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := c.cancelPending(ctx, order.ID, reason); err != nil {
		if errors.Is(err, errOrderNotCancelable) {
			return nil
		}
		return err
	}
	return nil
}
