package model

// 退款工单状态枚举（refund_tickets.status，开发计划 T15 / API 文档 11.4）。
//
// 状态机：
//
//	pending ──同意（微信退款 API 受理）──> approved ──退款回调成功──> refunded
//	pending / refund_failed ──同意但接口失败──> refund_failed（可重试）
//	pending ──驳回──> rejected（订单保持 paid）
//
// processing 预留给"已受理、处理中"的人工中间态；当前同步调用退款接口，
// 正常路径不产生该状态。
const (
	RefundTicketStatusPending      = "pending"
	RefundTicketStatusProcessing   = "processing"
	RefundTicketStatusApproved     = "approved"
	RefundTicketStatusRejected     = "rejected"
	RefundTicketStatusRefunded     = "refunded"
	RefundTicketStatusRefundFailed = "refund_failed"
)

// RefundTicketActiveStatuses 是同一订单只允许存在一条的进行中工单状态。
// rejected 不在此列：驳回后仍可再次申请（若未过报名截止）。
var RefundTicketActiveStatuses = []string{
	RefundTicketStatusPending,
	RefundTicketStatusProcessing,
	RefundTicketStatusApproved,
	RefundTicketStatusRefundFailed,
}

// RefundTicket 是退款工单（refund_tickets 表）。
type RefundTicket struct {
	ID           int64
	TicketNo     string
	OrderID      int64
	Reason       string
	Applicant    *string
	Status       string
	HandlerID    *int64 // 处理管理员
	RejectReason *string
	HandledAt    *string
	CreatedBy    *int64 // 登记人（客服/管理员，T15）
	CreatedAt    string
	UpdatedAt    string
}

// CreateRefundTicketRequest 是后台代录退款工单的请求体（T15）。
// applicant 缺省为「客服代录」。
type CreateRefundTicketRequest struct {
	OrderNo   string `json:"order_no" binding:"required"`
	Reason    string `json:"reason" binding:"required"`
	Applicant string `json:"applicant"`
}

// CreateRefundTicketResponse 是工单创建响应。
type CreateRefundTicketResponse struct {
	TicketID int64  `json:"ticket_id"`
	TicketNo string `json:"ticket_no"`
	Status   string `json:"status"`
}
