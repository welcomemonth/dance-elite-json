package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// maxOrderNoRetries 是 order_no 撞 UNIQUE 约束时的最大重生成次数。
const maxOrderNoRetries = 3

// RegistrationService 是报名下单服务（开发计划 T9，API 文档 3.1）。
//
// 提交报名 = 校验链 → 事务内创建 registration(pending_payment) + order(pending_payment)。
// 并发安全三防线（R1 定稿）：
//  1. 身份维度：registrations 部分唯一索引 (player_id, group_id)
//     WHERE status IN ('pending_payment','active')，同人同报名单元仅一条进行中报名；
//  2. 名额维度：TakeSlotTx 一条原子条件 UPDATE 抢名额（taken < quota AND enabled），
//     禁止"先 SELECT 再 UPDATE"的两步写法；
//  3. 请求幂等：orders (user_id, request_id) 唯一索引，重试复用同一 idempotency_key
//     直接返回首次创建的订单。
//
// 事务全程 BEGIN IMMEDIATE，依赖 SQLite 单写入者串行化。
//
// 安全约定：身份证明文只存在于请求参数与本函数栈内（PrepareIdentity 立即哈希），
// 不落库、不进日志、不进错误信息。
type RegistrationService struct {
	stores  *store.Store
	players *PlayerService
	now     func() time.Time
}

// NewRegistrationService 构造报名下单服务；players 为 T8 选手建档服务。
func NewRegistrationService(stores *store.Store, players *PlayerService) *RegistrationService {
	return &RegistrationService{stores: stores, players: players, now: time.Now}
}

// Submit 提交报名（API 文档 3.1）。返回的响应不含 pay_params——
// 支付参数由 T10 的 POST /orders/{id}/prepay 单独换取。
//
// 校验链依次执行，每步明确错误码：
// 赛事报名中（40401/41006）→ 身份证合法（41001）→ 年龄匹配组别（41002）
// → 组别开放（41007）→ [事务] 幂等查重（命中返回原订单）→ 一人一组别（41003）
// → 同舞种重复（41004）→ 原子抢名额（41005）。
func (s *RegistrationService) Submit(ctx context.Context, userID, eventID int64, req model.SubmitRegistrationRequest) (*model.SubmitRegistrationResponse, error) {
	// 1. 参数规范化与基础校验（40001）。
	name := strings.TrimSpace(req.PlayerName)
	if name == "" {
		return nil, httpx.ErrBadRequest("选手姓名不能为空")
	}
	if req.DanceType != model.DanceTypeClassical && req.DanceType != model.DanceTypeFolk {
		return nil, httpx.ErrBadRequest("舞种取值不合法，仅支持 classical / folk")
	}
	key := strings.TrimSpace(req.IdempotencyKey)
	if len(key) < 8 || len(key) > 64 || strings.ContainsAny(key, " \t\r\n") {
		return nil, httpx.ErrBadRequest("idempotency_key 不合法（8-64 位且不含空白字符）")
	}
	var institution *string
	if v := strings.TrimSpace(req.Institution); v != "" {
		institution = &v
	}

	// 2. 赛事可见性 + 报名期（40401/41006）。draft/unpublished/offline 按不可见处理（同 T7）。
	event, err := s.stores.Events.Get(ctx, eventID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !isPublishedVisible(event)) {
		return nil, httpx.ErrNotFound("赛事不存在或未发布")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
	}
	if deriveMiniappStatus(event, s.now()) != model.MiniappStatusRegistering {
		return nil, httpx.ErrNotInRegPeriod("该赛事当前不在报名期")
	}

	// 3. 身份校验：身份证合法性与出生日期解析（41001/40001）。
	// 明文在 PrepareIdentity 内立即哈希，返回的 prepared 不再含明文。
	prepared, err := s.players.PrepareIdentity(IdentityInput{
		IDType:   utils.IDTypeIDCard,
		IDNumber: req.IDCardNo,
		Name:     name,
	})
	if err != nil {
		return nil, err
	}
	birth, err := utils.ParseDate(*prepared.BirthDate)
	if err != nil {
		return nil, httpx.ErrInternal(err, "解析出生日期失败")
	}
	birthYear := birth.Year()

	// 4. 年龄匹配组别（41002）与启用校验（41007）。组别级别由出生年份唯一决定，
	// 同一年龄组别的另一舞种是合法报名（可报多个舞种，分开下单）。
	groups, err := s.stores.Groups.ListByEvent(ctx, eventID)
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载赛事组别失败，请稍后重试")
	}
	danceGroups := make([]model.EventGroup, 0, len(groups))
	for _, g := range groups {
		if g.DanceType == req.DanceType {
			danceGroups = append(danceGroups, g)
		}
	}
	if len(danceGroups) == 0 {
		return nil, httpx.ErrGroupDisabled("该赛事未开放所选舞种的报名")
	}
	matched := MatchGroupsByBirthYear(birthYear, danceGroups)
	if len(matched) == 0 {
		return nil, httpx.ErrAgeNotAllowed(fmt.Sprintf("不符合参赛年龄：选手出生于 %d 年，%s可报组别出生年份要求 %s",
			birthYear, danceTypeLabel(req.DanceType), describeGroupYearRanges(danceGroups)))
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	var target *model.EventGroup
	for i := range matched {
		if matched[i].Enabled {
			target = &matched[i]
			break
		}
	}
	if target == nil {
		return nil, httpx.ErrGroupDisabled(fmt.Sprintf("该组别（%s %s）已关闭报名", matched[0].Level, danceTypeLabel(req.DanceType)))
	}
	// 注：matched 多于一个说明组别年龄区间配置重叠（后台配置错误），
	// 上面已按 id 排序取首个，行为确定。

	// 5. 事务（BEGIN IMMEDIATE）：复核赛事 → 幂等查重 → 建档 → 判重 → 抢名额 → 写单。
	var resp *model.SubmitRegistrationResponse
	err = s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		// 5a. 事务内复核赛事仍在报名期：防"校验后、写单前"被后台关闭报名。
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
			return httpx.ErrNotInRegPeriod("该赛事当前不在报名期")
		}

		// 5b. 幂等查重：同人同 idempotency_key 直接返回首次创建的订单（R1 三防线之三）。
		// 唯一索引使查重永久生效（强于 API 文档"5 分钟内"的表述，重试安全性只增不减）。
		existing, err := s.stores.Orders.GetByRequestTx(ctx, tx, userID, key)
		if err != nil {
			return httpx.ErrInternal(err, "查询订单失败，请稍后重试")
		}
		if existing != nil {
			if existing.Order.Status != model.OrderStatusPendingPayment {
				// 订单已被取消/支付（T11/T12 路径）：同一幂等键不可复用，引导客户端换新键重新提交。
				return httpx.ErrConflict("该报名请求已结束，请重新提交报名")
			}
			resp = submitResponse(existing.Level, existing.DanceType, existing.MaterialCheckAt,
				existing.Order.ID, existing.Order.OrderNo, existing.Order.RegistrationID, existing.Order.Amount)
			return nil
		}

		// 5c. 匹配/建档选手（T8）：跨微信同证件号命中同一 player 档案。
		player, _, err := s.stores.Players.MatchOrCreate(ctx, tx, store.PlayerParams{
			Name:       prepared.Name,
			IDType:     prepared.IDType,
			IDCardHash: prepared.IDCardHash,
			Gender:     prepared.Gender,
			BirthDate:  prepared.BirthDate,
		})
		if err != nil {
			return httpx.ErrInternal(err, "创建选手档案失败，请稍后重试")
		}

		// 5d. 判重：先查一人一组别（41003，不同级别），再查同舞种重复（41004，同报名单元）。
		// 同一年龄组别的另一舞种是合法报名，直接放行进入抢名额。
		inFlights, err := s.stores.Registrations.InFlightByPlayerEventTx(ctx, tx, player.ID, eventID)
		if err != nil {
			return httpx.ErrInternal(err, "查询报名记录失败，请稍后重试")
		}
		for _, r := range inFlights {
			if r.Level != target.Level {
				return httpx.ErrLevelAlreadyTaken(fmt.Sprintf(
					"您已报名该赛事的 %s 组别（%s），同一赛事一人只能报名一个组别；可为该组别新增另一舞种的报名",
					r.Level, danceTypeLabel(r.DanceType)))
			}
		}
		for _, r := range inFlights {
			if r.GroupID == target.ID {
				return httpx.ErrDanceTypeDuplicate(fmt.Sprintf(
					"该赛事的 %s 组别已报名%s，请勿重复提交", target.Level, danceTypeLabel(target.DanceType)))
			}
		}

		// 5e. 原子抢名额（R1 三防线之二）：失败区分满员（41005，附组别信息引导候补）
		// 与关闭（41007）。
		took, snap, err := s.stores.Groups.TakeSlotTx(ctx, tx, target.ID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("赛事组别不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "占用名额失败，请稍后重试")
		}
		if !took {
			if !snap.Enabled {
				return httpx.ErrGroupDisabled(fmt.Sprintf("该组别（%s %s）已关闭报名", snap.Level, danceTypeLabel(snap.DanceType)))
			}
			return httpx.ErrQuotaFull("该组别名额已满，可提交候补信息",
				model.QuotaFullData{GroupID: snap.ID, Level: snap.Level, DanceType: snap.DanceType})
		}

		// 5f. 创建报名单 + 订单。任何一步失败整事务回滚（taken 由回滚自动回退）。
		regID, err := s.stores.Registrations.CreateTx(ctx, tx, store.RegistrationParams{
			EventID:     eventID,
			GroupID:     target.ID,
			PlayerID:    player.ID,
			UserID:      userID,
			DanceType:   req.DanceType,
			Institution: institution,
			Status:      model.RegistrationStatusPendingPayment,
		})
		if store.IsUniqueViolation(err) {
			// 并发下另一事务已先落同报名单元（唯一索引兜底，正常已被 5d 拦截）。
			return httpx.ErrDanceTypeDuplicate(fmt.Sprintf(
				"该赛事的 %s 组别已报名%s，请勿重复提交", target.Level, danceTypeLabel(target.DanceType)))
		}
		if err != nil {
			return httpx.ErrInternal(err, "创建报名单失败，请稍后重试")
		}

		orderID, orderNo, err := s.createOrderWithRetry(ctx, tx, userID, key, regID, ev.Fee)
		if err != nil {
			return err
		}

		resp = submitResponse(target.Level, target.DanceType, ev.MaterialCheckAt, orderID, orderNo, regID, ev.Fee)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// submitResponse 组装提交报名响应（API 文档 3.1，不含 pay_params）。
// order_id 供小程序直接调用预下单接口（POST /orders/{order_id}/prepay，T10）换取 pay_params。
func submitResponse(level, danceType string, materialDeadline *string, orderID int64, orderNo string, registrationID, amount int64) *model.SubmitRegistrationResponse {
	return &model.SubmitRegistrationResponse{
		OrderNo:          orderNo,
		OrderID:          orderID,
		RegistrationID:   registrationID,
		Amount:           amount,
		Level:            level,
		DanceType:        danceType,
		MaterialDeadline: materialDeadline,
	}
}

// createOrderWithRetry 在事务内生成订单号并创建订单：order_no 撞 UNIQUE 时换号重试；
// 同 request_id 冲突（已被幂等查重拦截的并发兜底）返回 40901。
func (s *RegistrationService) createOrderWithRetry(ctx context.Context, tx *sql.Tx, userID int64, requestID string, registrationID, amount int64) (int64, string, error) {
	var lastErr error
	for i := 0; i < maxOrderNoRetries; i++ {
		orderNo, err := generateOrderNo(s.now())
		if err != nil {
			return 0, "", httpx.ErrInternal(err, "生成订单号失败，请稍后重试")
		}
		orderID, err := s.stores.Orders.CreateTx(ctx, tx, store.OrderParams{
			OrderNo:        orderNo,
			RegistrationID: registrationID,
			UserID:         userID,
			Amount:         amount,
			Status:         model.OrderStatusPendingPayment,
			RequestID:      &requestID,
		})
		if err == nil {
			return orderID, orderNo, nil
		}
		lastErr = err
		if !store.IsUniqueViolation(err) {
			return 0, "", httpx.ErrInternal(err, "创建订单失败，请稍后重试")
		}
		// 唯一冲突：区分 order_no 撞库（换号重试）与 request_id 并发兜底（40901）。
		var n int
		if qErr := tx.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM orders WHERE user_id = ? AND request_id = ?`, userID, requestID).Scan(&n); qErr == nil && n > 0 {
			return 0, "", httpx.ErrConflict("该报名请求正在处理中，请稍后重试")
		}
	}
	return 0, "", httpx.ErrInternal(lastErr, "创建订单失败，请稍后重试")
}

// danceTypeLabel 返回舞种中文名（用户可见文案）。
func danceTypeLabel(dt string) string {
	switch dt {
	case model.DanceTypeClassical:
		return "古典舞"
	case model.DanceTypeFolk:
		return "民族民间舞"
	default:
		return dt
	}
}

// generateOrderNo 生成全局唯一且不可推测的订单号：14 位紧凑时间（+08:00）
// + 8 位 crypto/rand 数字（如 2027010109300012345678，API 文档 3.1）。
// 唯一性由 orders.order_no UNIQUE 兜底，撞库时由调用方重试。
func generateOrderNo(now time.Time) (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	digits := make([]byte, len(buf))
	for i, b := range buf {
		digits[i] = '0' + b%10
	}
	return now.In(utils.ChinaLoc).Format("20060102150405") + string(digits), nil
}
