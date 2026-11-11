package service

import (
	"fmt"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// checkRefundEligibility 校验订单是否允许进入退款流程（开发计划 T15）。
//
// 规则（API 文档 11.4 注意事项）：
//   - 订单必须为 paid：待支付/已取消无款可退；退款中/已退款不可重复申请；
//   - 报名截止（reg_end_at）之后不再受理：到达截止时刻即视为截止（与
//     deriveMiniappStatus 的注册关闭口径一致）。
//
// 时间线缺失/格式异常属数据问题，返回普通 error 由路由映射为 50001 并记日志。
func checkRefundEligibility(order *model.Order, event *model.Event, now time.Time) error {
	switch order.Status {
	case model.OrderStatusPaid:
		// 正常路径。
	case model.OrderStatusPendingPayment:
		return httpx.ErrRefundNotAllowed("订单尚未支付，无需退款")
	case model.OrderStatusCanceled:
		return httpx.ErrRefundNotAllowed("订单已取消，无可退款项")
	case model.OrderStatusRefunding:
		return httpx.ErrRefundNotAllowed("订单退款处理中，请勿重复申请")
	case model.OrderStatusRefunded:
		return httpx.ErrRefundNotAllowed("订单已退款，请勿重复申请")
	default:
		return httpx.ErrRefundNotAllowed("订单当前状态不允许退款")
	}

	if event.RegEndAt == nil || *event.RegEndAt == "" {
		return fmt.Errorf("赛事 %d 缺少报名截止时间，无法核验退款规则", event.ID)
	}
	regEnd, err := utils.ParseISO(*event.RegEndAt)
	if err != nil {
		return fmt.Errorf("解析赛事 %d 报名截止时间: %w", event.ID, err)
	}
	if !now.Before(regEnd) {
		return httpx.ErrRefundNotAllowed("已过报名截止时间，不再受理退款申请")
	}
	return nil
}
