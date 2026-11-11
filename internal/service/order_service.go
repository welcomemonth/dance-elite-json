package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/wxpay"
	"github.com/meetmonth/dance-elite/internal/store"
)

// OrderService 是订单支付服务（开发计划 T10，API 文档 3.1b）。
//
// Prepay 流程：归属与状态校验 → 已有 prepay_id 直接复用（幂等）→
// 调网关统一下单 → 条件写入 prepay_id（并发只有一个胜出）→ 签名支付参数。
// 同一订单的预下单在进程内按订单串行化：真实通道下并发重复下单会被微信
// 以 out_trade_no 重复拒绝，单机部署内用按订单锁规避。
type OrderService struct {
	stores  *store.Store
	gateway wxpay.Gateway

	mu    sync.Mutex
	locks map[int64]*sync.Mutex // 按订单的预下单串行锁（惰创建）
}

// NewOrderService 构造订单支付服务；gateway 为 wxpay.Client（真实）或
// wxpay.MockGateway（PAY_MOCK=true）。
func NewOrderService(stores *store.Store, gateway wxpay.Gateway) *OrderService {
	return &OrderService{stores: stores, gateway: gateway, locks: make(map[int64]*sync.Mutex)}
}

// orderLock 返回指定订单的串行锁（map 访问与锁创建受 mu 保护）。
func (s *OrderService) orderLock(orderID int64) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.locks[orderID]
	if !ok {
		l = &sync.Mutex{}
		s.locks[orderID] = l
	}
	return l
}

// Prepay 为订单换取小程序支付参数（POST /orders/{order_id}/prepay）。
// openid 取自登录 JWT（T4 payload），无需再查库。
// 错误码：40401 订单不存在或非本人；40901 订单已支付/已取消（无需重复支付）；
// 50001 微信网关失败（细节进日志，用户文案固定）。
func (s *OrderService) Prepay(ctx context.Context, userID int64, openid string, orderID int64) (*model.PrepayResponse, error) {
	lock := s.orderLock(orderID)
	lock.Lock()
	defer lock.Unlock()

	order, err := s.stores.Orders.Get(ctx, orderID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && order.UserID != userID) {
		return nil, httpx.ErrNotFound("订单不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载订单失败，请稍后重试")
	}
	if order.Status != model.OrderStatusPendingPayment {
		return nil, httpx.ErrConflict("订单已支付或已取消，无需重复支付")
	}

	// 幂等：已换取过 prepay_id 的订单直接复用（网络重试/用户重复点击安全）。
	prepayID := derefString(order.PrepayID)
	if prepayID == "" {
		prepayID, err = s.placePrepay(ctx, order, openid)
		if err != nil {
			return nil, err
		}
		if err := s.persistPrepayID(ctx, order.ID, prepayID); err != nil {
			return nil, err
		}
	}

	params, err := s.gateway.SignPayParams(prepayID)
	if err != nil {
		return nil, httpx.ErrInternal(err, "生成支付参数失败，请稍后重试")
	}
	return &model.PrepayResponse{
		OrderNo:   order.OrderNo,
		PayParams: model.WxPayParams(params),
	}, nil
}

// placePrepay 调支付网关统一下单；微信业务错误与网络错误统一映射为
// 50001（细节含在 Cause 里进日志，用户文案固定，避免泄露微信侧细节）。
func (s *OrderService) placePrepay(ctx context.Context, order *model.Order, openid string) (string, error) {
	description, err := s.payDescription(ctx, order.RegistrationID)
	if err != nil {
		return "", err
	}
	prepayID, err := s.gateway.Prepay(ctx, wxpay.PrepayRequest{
		OpenID:      openid,
		OutTradeNo:  order.OrderNo,
		Description: description,
		AmountFen:   order.Amount,
	})
	if err != nil {
		return "", httpx.ErrInternal(err, "拉起支付失败，请稍后重试")
	}
	if prepayID == "" {
		return "", httpx.ErrInternal(errors.New("empty prepay_id"), "拉起支付失败，请稍后重试")
	}
	return prepayID, nil
}

// payDescription 取赛事名作为商品描述（展示在微信支付记录中）；
// 赛事缺失等异常退化为通用文案，不阻断支付。
func (s *OrderService) payDescription(ctx context.Context, registrationID int64) (string, error) {
	registration, err := s.stores.Registrations.Get(ctx, registrationID)
	if errors.Is(err, store.ErrNotFound) {
		return "优舞盟赛事报名费", nil
	}
	if err != nil {
		return "", httpx.ErrInternal(err, "加载报名信息失败，请稍后重试")
	}
	event, err := s.stores.Events.Get(ctx, registration.EventID)
	if errors.Is(err, store.ErrNotFound) {
		return "优舞盟赛事报名费", nil
	}
	if err != nil {
		return "", httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
	}
	return fmt.Sprintf("优舞盟报名费-%s", event.Name), nil
}

// persistPrepayID 条件写入 prepay_id（仅 pending_payment 且未写入过）。
// 并发场景下未命中说明另一请求已写入：回读订单，复用其 prepay_id；
// 若订单状态已变（取消/支付），返回 40901。
func (s *OrderService) persistPrepayID(ctx context.Context, orderID int64, prepayID string) error {
	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		return s.stores.Orders.SetPrepayIDTx(ctx, tx, orderID, prepayID)
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, store.ErrConflict) {
		return httpx.ErrInternal(err, "保存支付单号失败，请稍后重试")
	}
	// 并发落败：回读确认订单状态后复用胜出的 prepay_id 或报冲突。
	order, getErr := s.stores.Orders.Get(ctx, orderID)
	if getErr != nil {
		return httpx.ErrInternal(getErr, "加载订单失败，请稍后重试")
	}
	if order.Status != model.OrderStatusPendingPayment {
		return httpx.ErrConflict("订单已支付或已取消，无需重复支付")
	}
	if derefString(order.PrepayID) == "" {
		// 非并发落败且 prepay_id 仍空：视为写入异常，向上抛 50001。
		return httpx.ErrInternal(err, "保存支付单号失败，请稍后重试")
	}
	return nil
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
