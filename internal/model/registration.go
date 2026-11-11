package model

// 报名单状态枚举（registrations.status，开发计划 T9 定稿）：
// 提交报名即插入 pending_payment，支付回调转 active（T11），取消转 canceled（T12），
// 退款退赛转 withdrawn（T15）。
const (
	RegistrationStatusPendingPayment = "pending_payment"
	RegistrationStatusActive         = "active"
	RegistrationStatusCanceled       = "canceled"
	RegistrationStatusWithdrawn      = "withdrawn"
)

// RegistrationInFlightStatuses 是"进行中"报名状态集合，与部分唯一索引
// uq_registrations_in_flight 的 WHERE 条件保持一致（判重/查重口径）。
var RegistrationInFlightStatuses = []string{
	RegistrationStatusPendingPayment,
	RegistrationStatusActive,
}

// 订单状态枚举（orders.status）。pending_payment 为中间态（前端不可见），
// 创建订单即插入；终态：paid / canceled / refunded。
const (
	OrderStatusPendingPayment = "pending_payment"
	OrderStatusPaid           = "paid"
	OrderStatusCanceled       = "canceled"
	OrderStatusRefunding      = "refunding"
	OrderStatusRefunded       = "refunded"
)

// 订单取消原因枚举（orders.cancel_reason，T12 使用）。
const (
	OrderCancelReasonUserCancel = "user_cancel"
	OrderCancelReasonPayFailed  = "pay_failed"
	OrderCancelReasonSystem     = "system"
)

// Registration 是报名单（registrations 表）。一次报名 = 选手 × 赛事 × 组别 × 舞种；
// 机构归属报名单（选手不同场次可代表不同机构）。8 项参赛资料字段直接挂在报名单上。
type Registration struct {
	ID                int64
	EventID           int64
	GroupID           int64
	PlayerID          int64
	UserID            int64
	DanceType         string
	Institution       *string
	Status            string
	Source            string
	CSTicketID        *int64
	PhotoPath         *string
	AudioPath         *string
	ProgramName       *string
	TeacherName       *string
	ContactPhone      *string
	ExpressAddress    *string
	WorkIntro         *string
	SpecialReq        *string
	MaterialUpdatedAt *string
	CreatedAt         string
	UpdatedAt         string
}

// Order 是订单（orders 表）。订单与报名单 1:1（一次报名一笔订单）。
type Order struct {
	ID             int64
	OrderNo        string
	RegistrationID int64
	UserID         int64
	Amount         int64
	Status         string
	RequestID      *string
	PaidAt         *string
	CanceledAt     *string
	CancelReason   *string
	CreatedAt      string
	UpdatedAt      string
}

// SubmitRegistrationRequest 是提交报名（POST /api/v1/events/{event_id}/registrations，
// API 文档 3.1）的请求体。id_card_no 仅本次传输使用明文，服务端立即哈希，不落明文、不进日志。
type SubmitRegistrationRequest struct {
	PlayerName     string `json:"player_name" binding:"required,max=30"`
	IDCardNo       string `json:"id_card_no" binding:"required"`
	Institution    string `json:"institution" binding:"omitempty,max=50"`
	DanceType      string `json:"dance_type" binding:"required"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
}

// SubmitRegistrationResponse 是提交报名的响应（API 文档 3.1）。
// pay_params 不在本接口返回：T10 起由 POST /orders/{id}/prepay 单独换取
// （开发计划 T9/T10 拆分，见开发计划 T9 落地补充）。
type SubmitRegistrationResponse struct {
	OrderNo          string  `json:"order_no"`
	RegistrationID   int64   `json:"registration_id"`
	Amount           int64   `json:"amount"`
	Level            string  `json:"level"`
	DanceType        string  `json:"dance_type"`
	MaterialDeadline *string `json:"material_deadline"`
}

// QuotaFullData 是 41005（名额已满）错误响应 data 载荷（API 文档 3.1），
// 前端据此预填候补表单。
type QuotaFullData struct {
	GroupID   int64  `json:"group_id"`
	Level     string `json:"level"`
	DanceType string `json:"dance_type"`
}
