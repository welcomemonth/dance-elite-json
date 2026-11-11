package model

// AdminOrderListQuery 是后台订单列表筛选（API 文档 11.1）。
// 默认不返回 pending_payment（支付中间态）与 canceled（对账留档）；
// IncludeCanceled=true 时含已取消订单，Status 显式指定时以 Status 为准。
type AdminOrderListQuery struct {
	EventID         int64
	Level           string
	DanceType       string
	Status          string
	IncludeCanceled bool
	Source          string
	PaidFrom        string // ISO8601（服务端归一化为全库时间格式）
	PaidTo          string
	Keyword         string // 姓名 / 订单号 / 微信支付单号
	Page            int
	PageSize        int
}

// AdminOrderItem 是后台订单列表单条记录。
// 系统只存证件号 HMAC 哈希（决策 #1），列表以 player_no + id_type 标识选手，
// 不返回任何形态的证件号。
type AdminOrderItem struct {
	OrderNo       string  `json:"order_no"`
	TransactionID *string `json:"transaction_id"`
	EventID       int64   `json:"event_id"`
	EventName     string  `json:"event_name"`
	Level         string  `json:"level"`
	DanceType     string  `json:"dance_type"`
	PlayerNo      string  `json:"player_no"`
	PlayerName    string  `json:"player_name"`
	IDType        string  `json:"id_type"`
	Amount        int64   `json:"amount"`
	Status        string  `json:"status"`
	Source        string  `json:"source"`
	PaidAt        *string `json:"paid_at"`
	CreatedAt     string  `json:"created_at"`
}

// AdminOrderListResponse 是后台订单列表响应（统一分页结构）。
type AdminOrderListResponse struct {
	List     []AdminOrderItem `json:"list"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}
