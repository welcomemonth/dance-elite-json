package model

// AdminOrderDetailOrder 是订单详情中的订单主信息。
type AdminOrderDetailOrder struct {
	OrderNo        string  `json:"order_no"`
	TransactionID  *string `json:"transaction_id"`
	RegistrationID int64   `json:"registration_id"`
	EventID        int64   `json:"event_id"`
	EventName      string  `json:"event_name"`
	Level          string  `json:"level"`
	DanceType      string  `json:"dance_type"`
	PlayerNo       string  `json:"player_no"`
	PlayerName     string  `json:"player_name"`
	IDType         string  `json:"id_type"`
	Amount         int64   `json:"amount"`
	Status         string  `json:"status"`
	Source         string  `json:"source"`
	PaidAt         *string `json:"paid_at"`
	CanceledAt     *string `json:"canceled_at"`
	CancelReason   *string `json:"cancel_reason"`
	CreatedAt      string  `json:"created_at"`
}

// AdminOrderDetailRegistration 是订单详情中的报名信息。
type AdminOrderDetailRegistration struct {
	RegistrationID int64   `json:"registration_id"`
	EventID        int64   `json:"event_id"`
	GroupID        int64   `json:"group_id"`
	Level          string  `json:"level"`
	DanceType      string  `json:"dance_type"`
	PlayerNo       string  `json:"player_no"`
	PlayerName     string  `json:"player_name"`
	IDType         string  `json:"id_type"`
	Institution    *string `json:"institution"`
	ContactPhone   *string `json:"contact_phone"`
	Status         string  `json:"status"`
	Source         string  `json:"source"`
}

// AdminOrderPaymentItem 是订单详情中的一条支付/退款流水。
type AdminOrderPaymentItem struct {
	PaymentID     int64   `json:"payment_id"`
	Type          string  `json:"type"`
	PrepayID      *string `json:"prepay_id"`
	TransactionID *string `json:"transaction_id"`
	RefundID      *string `json:"refund_id"`
	Amount        int64   `json:"amount"`
	Status        string  `json:"status"`
	NotifiedAt    *string `json:"notified_at"`
	CreatedAt     string  `json:"created_at"`
}

// AdminOrderRefundTicketItem 是订单关联的退款工单摘要。
type AdminOrderRefundTicketItem struct {
	TicketID      int64   `json:"ticket_id"`
	TicketNo      string  `json:"ticket_no"`
	Status        string  `json:"status"`
	Reason        string  `json:"reason"`
	Applicant     *string `json:"applicant"`
	RejectReason  *string `json:"reject_reason"`
	CreatedByName *string `json:"created_by_name"`
	HandlerName   *string `json:"handler_name"`
	HandledAt     *string `json:"handled_at"`
	CreatedAt     string  `json:"created_at"`
}

// AdminOrderCSTicketItem 是订单关联的客服工单摘要。
type AdminOrderCSTicketItem struct {
	TicketID              int64   `json:"ticket_id"`
	Type                  string  `json:"type"`
	Status                string  `json:"status"`
	UserName              *string `json:"user_name"`
	Phone                 string  `json:"phone"`
	Content               string  `json:"content"`
	RelatedRegistrationID *int64  `json:"related_registration_id"`
	Remark                *string `json:"remark"`
	CreatedAt             string  `json:"created_at"`
	UpdatedAt             string  `json:"updated_at"`
}

// AdminOrderMaterials 是订单详情中的资料区（快照 + 截止只读标记）。
type AdminOrderMaterials struct {
	MaterialsSnapshot
	Readonly bool `json:"readonly"`
}

// AdminOperationLogItem 是一条状态流转记录（由订单/流水/工单时间线合成）。
type AdminOperationLogItem struct {
	Action     string  `json:"action"`
	FromStatus *string `json:"from_status"`
	ToStatus   *string `json:"to_status"`
	Operator   string  `json:"operator"`
	CreatedAt  string  `json:"created_at"`
}

// AdminOrderDetailResponse 是订单详情响应（API 文档 11.2）：
// 报名/支付/资料/退款工单/客服工单/操作记录。
type AdminOrderDetailResponse struct {
	Order          AdminOrderDetailOrder        `json:"order"`
	Registration   AdminOrderDetailRegistration `json:"registration"`
	Payments       []AdminOrderPaymentItem      `json:"payments"`
	RefundTickets  []AdminOrderRefundTicketItem `json:"refund_tickets"`
	CSTickets      []AdminOrderCSTicketItem     `json:"cs_tickets"`
	Materials      AdminOrderMaterials          `json:"materials"`
	MaterialStatus MaterialStatus               `json:"material_status"`
	OperationLogs  []AdminOperationLogItem      `json:"operation_logs"`
}
