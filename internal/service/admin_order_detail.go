package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// Detail 返回订单详情（API 文档 11.2）：订单/报名/支付流水/退款工单/客服工单/
// 资料快照/状态流转记录。证件号只有 HMAC 哈希，详情同样以 player_no 标识选手。
func (s *AdminOrderQueryService) Detail(ctx context.Context, orderNo string) (*model.AdminOrderDetailResponse, error) {
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return nil, httpx.ErrBadRequest("订单号不能为空")
	}
	row, err := s.stores.Orders.GetAdminDetail(ctx, orderNo)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpx.ErrNotFound("订单不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载订单详情失败，请稍后重试")
	}

	payments, err := s.stores.Payments.ListByOrder(ctx, row.Order.ID)
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载支付流水失败，请稍后重试")
	}
	tickets, err := s.stores.RefundTickets.ListByOrder(ctx, row.Order.ID)
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载退款工单失败，请稍后重试")
	}
	csTickets, err := s.stores.CSTickets.ListByOrderOrRegistration(ctx, row.Order.ID, row.Registration.ID)
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载客服工单失败，请稍后重试")
	}

	resp := &model.AdminOrderDetailResponse{
		Order: model.AdminOrderDetailOrder{
			OrderNo:        row.Order.OrderNo,
			TransactionID:  row.TransactionID,
			RegistrationID: row.Order.RegistrationID,
			EventID:        row.Registration.EventID,
			EventName:      row.EventName,
			Level:          row.Level,
			DanceType:      row.Registration.DanceType,
			PlayerNo:       row.PlayerNo,
			PlayerName:     row.PlayerName,
			IDType:         row.IDType,
			Amount:         row.Order.Amount,
			Status:         row.Order.Status,
			Source:         row.Order.Source,
			PaidAt:         row.Order.PaidAt,
			CanceledAt:     row.Order.CanceledAt,
			CancelReason:   row.Order.CancelReason,
			CreatedAt:      row.Order.CreatedAt,
		},
		Registration: model.AdminOrderDetailRegistration{
			RegistrationID: row.Registration.ID,
			EventID:        row.Registration.EventID,
			GroupID:        row.Registration.GroupID,
			Level:          row.Level,
			DanceType:      row.Registration.DanceType,
			PlayerNo:       row.PlayerNo,
			PlayerName:     row.PlayerName,
			IDType:         row.IDType,
			Institution:    row.Registration.Institution,
			ContactPhone:   row.ContactPhone,
			Status:         row.Registration.Status,
			Source:         row.Registration.Source,
		},
		Payments:      paymentItems(payments),
		RefundTickets: refundTicketItems(tickets),
		CSTickets:     csTicketItems(csTickets),
		Materials: model.AdminOrderMaterials{
			MaterialsSnapshot: MaterialsSnapshotOf(&row.Registration),
			Readonly:          materialReadonly(row.MaterialCheckAt, s.now()),
		},
		MaterialStatus: MaterialStatusOf(&row.Registration),
		OperationLogs:  operationLogs(row, tickets),
	}
	return resp, nil
}

// materialReadonly 判断资料是否已过上传截止时间（material_check_at）。
func materialReadonly(materialCheckAt *string, now time.Time) bool {
	if materialCheckAt == nil || *materialCheckAt == "" {
		return false
	}
	return utils.FormatISO(now) >= *materialCheckAt
}

// paymentItems 映射支付/退款流水。
func paymentItems(payments []model.Payment) []model.AdminOrderPaymentItem {
	list := make([]model.AdminOrderPaymentItem, 0, len(payments))
	for _, p := range payments {
		list = append(list, model.AdminOrderPaymentItem{
			PaymentID:     p.ID,
			Type:          p.Type,
			PrepayID:      p.PrepayID,
			TransactionID: p.TransactionID,
			RefundID:      p.RefundID,
			Amount:        p.Amount,
			Status:        p.Status,
			NotifiedAt:    p.NotifiedAt,
			CreatedAt:     p.CreatedAt,
		})
	}
	return list
}

// refundTicketItems 映射退款工单摘要。
func refundTicketItems(rows []store.AdminRefundTicketRow) []model.AdminOrderRefundTicketItem {
	list := make([]model.AdminOrderRefundTicketItem, 0, len(rows))
	for _, r := range rows {
		list = append(list, model.AdminOrderRefundTicketItem{
			TicketID:      r.Ticket.ID,
			TicketNo:      r.Ticket.TicketNo,
			Status:        r.Ticket.Status,
			Reason:        r.Ticket.Reason,
			Applicant:     r.Ticket.Applicant,
			RejectReason:  r.Ticket.RejectReason,
			CreatedByName: r.CreatedByName,
			HandlerName:   r.HandlerName,
			HandledAt:     r.Ticket.HandledAt,
			CreatedAt:     r.Ticket.CreatedAt,
		})
	}
	return list
}

// csTicketItems 映射客服工单摘要。
func csTicketItems(tickets []model.CSTicket) []model.AdminOrderCSTicketItem {
	list := make([]model.AdminOrderCSTicketItem, 0, len(tickets))
	for _, t := range tickets {
		list = append(list, model.AdminOrderCSTicketItem{
			TicketID:              t.ID,
			Type:                  t.Type,
			Status:                t.Status,
			UserName:              t.UserName,
			Phone:                 t.Phone,
			Content:               t.Content,
			RelatedRegistrationID: t.RelatedRegistrationID,
			Remark:                t.Remark,
			CreatedAt:             t.CreatedAt,
			UpdatedAt:             t.UpdatedAt,
		})
	}
	return list
}

// operationLogs 由订单时间线与退款工单状态合成状态流转记录，按时间升序。
func operationLogs(row *store.AdminOrderDetailRow, tickets []store.AdminRefundTicketRow) []model.AdminOperationLogItem {
	order := row.Order
	from := model.OrderStatusPendingPayment
	logs := make([]model.AdminOperationLogItem, 0, 3+len(tickets))
	logs = append(logs, model.AdminOperationLogItem{
		Action:    "created",
		ToStatus:  &from,
		Operator:  orderSourceOperator(order.Source),
		CreatedAt: order.CreatedAt,
	})
	pending := model.OrderStatusPendingPayment
	if order.PaidAt != nil {
		paid := model.OrderStatusPaid
		logs = append(logs, model.AdminOperationLogItem{
			Action: "paid", FromStatus: &pending, ToStatus: &paid,
			Operator: orderPaymentOperator(order.Source), CreatedAt: *order.PaidAt,
		})
	}
	if order.CanceledAt != nil && order.CancelReason != nil {
		canceled := model.OrderStatusCanceled
		logs = append(logs, model.AdminOperationLogItem{
			Action: "canceled", FromStatus: &pending, ToStatus: &canceled,
			Operator: cancelReasonOperator(*order.CancelReason), CreatedAt: *order.CanceledAt,
		})
	}
	for _, r := range tickets {
		logs = append(logs, refundTicketLogs(r)...)
	}
	sort.SliceStable(logs, func(i, j int) bool { return logs[i].CreatedAt < logs[j].CreatedAt })
	return logs
}

// refundTicketLogs 把单张退款工单映射为申请 + 处理结果两条流转记录。
func refundTicketLogs(r store.AdminRefundTicketRow) []model.AdminOperationLogItem {
	pending := model.RefundTicketStatusPending
	logs := []model.AdminOperationLogItem{{
		Action:    "refund_requested",
		ToStatus:  &pending,
		Operator:  ptrValue(r.CreatedByName, "客服"),
		CreatedAt: r.Ticket.CreatedAt,
	}}
	if r.Ticket.HandledAt == nil {
		return logs
	}
	status := r.Ticket.Status
	action := "refund_" + status
	logs = append(logs, model.AdminOperationLogItem{
		Action:    action,
		ToStatus:  &status,
		Operator:  ptrValue(r.HandlerName, "系统"),
		CreatedAt: *r.Ticket.HandledAt,
	})
	return logs
}

// orderSourceOperator 返回订单创建方描述。
func orderSourceOperator(source string) string {
	if source == model.OrderSourceManual {
		return "后台人工"
	}
	return "用户"
}

// orderPaymentOperator 返回订单支付方描述。
func orderPaymentOperator(source string) string {
	if source == model.OrderSourceManual {
		return "后台人工"
	}
	return "微信支付"
}

// cancelReasonOperator 把取消原因映射为操作方描述。
func cancelReasonOperator(reason string) string {
	switch reason {
	case model.OrderCancelReasonUserCancel:
		return "用户"
	case model.OrderCancelReasonPayFailed:
		return "系统（支付失败）"
	default:
		return "系统"
	}
}

// ptrValue 取指针值，空指针返回 fallback。
func ptrValue(p *string, fallback string) string {
	if p == nil || *p == "" {
		return fallback
	}
	return *p
}
