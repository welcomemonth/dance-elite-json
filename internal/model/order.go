package model

// WxPayParams 是微信小程序支付参数（API 文档 3.1 pay_params）。
// 字段名遵循微信 wx.requestPayment 协议约定（camelCase），是本项目
// snake_case 命名规则的文档级例外。
type WxPayParams struct {
	TimeStamp string `json:"timeStamp"`
	NonceStr  string `json:"nonceStr"`
	Package   string `json:"package"`
	SignType  string `json:"signType"`
	PaySign   string `json:"paySign"`
}

// PrepayResponse 是 POST /api/v1/orders/{order_id}/prepay 的响应
// （API 文档 3.1b，T10）。
type PrepayResponse struct {
	OrderNo   string      `json:"order_no"`
	PayParams WxPayParams `json:"pay_params"`
}

// OrderStatusResponse 是 GET /api/v1/orders/{order_id} 的响应
// （API 文档 3.1c，T11 订单状态查询与主动查单兜底）。
type OrderStatusResponse struct {
	OrderID        int64   `json:"order_id"`
	OrderNo        string  `json:"order_no"`
	Status         string  `json:"status"`
	Amount         int64   `json:"amount"`
	PaidAt         *string `json:"paid_at"`
	RegistrationID int64   `json:"registration_id"`
}

// CancelOrderRequest 是 POST /api/v1/orders/{order_id}/cancel 的请求体（T12）。
// reason 可选，缺省 user_cancel；支付失败时客户端可传 pay_failed。
// system 仅供服务端内部使用，不接受客户端提交。
type CancelOrderRequest struct {
	Reason string `json:"reason"`
}

// CancelOrderResponse 是取消订单的响应（T12）：取消后订单留档只读。
type CancelOrderResponse struct {
	OrderID      int64   `json:"order_id"`
	OrderNo      string  `json:"order_no"`
	Status       string  `json:"status"`
	CancelReason *string `json:"cancel_reason"`
	CanceledAt   *string `json:"canceled_at"`
}

// OrderHistoryItem 是「我的-历史订单」单条记录（T12，GET /api/v1/orders）。
// 含已取消订单（只读留档），不含待支付订单（支付流程中间态不展示）。
type OrderHistoryItem struct {
	OrderID        int64   `json:"order_id"`
	OrderNo        string  `json:"order_no"`
	RegistrationID int64   `json:"registration_id"`
	EventID        int64   `json:"event_id"`
	EventName      string  `json:"event_name"`
	LevelGroup     string  `json:"level_group"`
	DanceType      string  `json:"dance_type"`
	PlayerName     string  `json:"player_name"`
	Amount         int64   `json:"amount"`
	Status         string  `json:"status"`
	CancelReason   *string `json:"cancel_reason"`
	PaidAt         *string `json:"paid_at"`
	CanceledAt     *string `json:"canceled_at"`
	CreatedAt      string  `json:"created_at"`
}

// OrderHistoryListResponse 是历史订单列表响应（统一分页结构）。
type OrderHistoryListResponse struct {
	List     []OrderHistoryItem `json:"list"`
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}
