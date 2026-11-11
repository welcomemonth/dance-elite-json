package model

// 支付流水类型（payments.type）：支付 / 退款。
const (
	PaymentTypePay    = "pay"
	PaymentTypeRefund = "refund"
)

// 支付流水状态（payments.status）。
const (
	PaymentStatusSuccess = "success"
	PaymentStatusFailed  = "failed"
	PaymentStatusClosed  = "closed"
)

// Payment 是支付流水（payments 表），对账与回调幂等的唯一事实来源。
// transaction_id 唯一约束保证重复支付回调只落一条流水（T11）；
// refund_id 部分唯一索引供 T15 退款回调幂等使用。
type Payment struct {
	ID            int64
	OrderID       int64
	Type          string
	PrepayID      *string
	TransactionID *string
	RefundID      *string
	Amount        int64
	Status        string
	RawCallback   *string // 原始回调报文（仅支付回调写入；查单兜底为 NULL）
	NotifiedAt    *string // 回调处理时间
	CreatedAt     string
	UpdatedAt     string
}
