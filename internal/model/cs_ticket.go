package model

// 客服工单类型（cs_tickets.type，API 文档 13.3）。
const (
	CSTicketTypeRefund   = "refund"     // 退款诉求
	CSTicketTypeNoIDCard = "no_id_card" // 无证件号报名登记
	CSTicketTypeWaitlist = "waitlist"   // 候补咨询
	CSTicketTypeOther    = "other"      // 其他
)

// 客服工单状态（cs_tickets.status）：pending → processing → done。
const (
	CSTicketStatusPending    = "pending"
	CSTicketStatusProcessing = "processing"
	CSTicketStatusDone       = "done"
)

// CSTicket 是客服工单（cs_tickets 表）。客服登记退款诉求、无证件号报名等，
// 管理员处理后流转；关联订单/报名单可选，人工报名创建后回填 related_registration_id。
type CSTicket struct {
	ID                    int64
	Type                  string
	UserName              *string
	Phone                 string
	Content               string
	RelatedOrderID        *int64
	RelatedRegistrationID *int64
	Status                string
	Remark                *string
	CreatedBy             *int64
	CreatedAt             string
	UpdatedAt             string
}

// CreateCSTicketRequest 是登记客服工单的请求体（API 文档 13.3，POST）。
// related_order_no 为商户订单号，服务端解析为 related_order_id。
type CreateCSTicketRequest struct {
	Type           string `json:"type" binding:"required"`
	UserName       string `json:"user_name" binding:"omitempty,max=50"`
	Phone          string `json:"phone" binding:"required"`
	Content        string `json:"content" binding:"required"`
	RelatedOrderNo string `json:"related_order_no"`
}

// CreateCSTicketResponse 是登记工单的响应。
type CreateCSTicketResponse struct {
	TicketID int64  `json:"ticket_id"`
	Status   string `json:"status"`
}

// UpdateCSTicketRequest 是工单流转请求体（API 文档 13.3，PUT）：
// status 只能前进（pending → processing → done），remark 覆盖为最新跟进备注。
type UpdateCSTicketRequest struct {
	Status string `json:"status" binding:"required"`
	Remark string `json:"remark" binding:"omitempty,max=500"`
}

// UpdateCSTicketResponse 是工单流转的响应。
type UpdateCSTicketResponse struct {
	TicketID int64  `json:"ticket_id"`
	Status   string `json:"status"`
}

// AdminCSTicketListQuery 是后台工单列表筛选（type/status + 分页）。
// CreatedBy 由服务端按角色填充：cs 只看自己登记的，admin 看全量。
type AdminCSTicketListQuery struct {
	Type      string
	Status    string
	CreatedBy int64
	Page      int
	PageSize  int
}

// AdminCSTicketItem 是后台工单列表单条记录。
type AdminCSTicketItem struct {
	TicketID              int64   `json:"ticket_id"`
	Type                  string  `json:"type"`
	UserName              *string `json:"user_name"`
	Phone                 string  `json:"phone"`
	Content               string  `json:"content"`
	RelatedOrderNo        *string `json:"related_order_no"`
	RelatedRegistrationID *int64  `json:"related_registration_id"`
	Status                string  `json:"status"`
	Remark                *string `json:"remark"`
	CreatedBy             *int64  `json:"created_by"`
	CreatedByName         *string `json:"created_by_name"`
	CreatedAt             string  `json:"created_at"`
	UpdatedAt             string  `json:"updated_at"`
}

// AdminCSTicketListResponse 是工单列表响应（统一分页结构）。
type AdminCSTicketListResponse struct {
	List     []AdminCSTicketItem `json:"list"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}
