package model

// 候补状态枚举（waitlists.status，API 文档 A.3 状态机）：
//
//	waiting --管理员电话通知--> notified --用户完成支付--> converted
//	waiting/notified --用户放弃/管理员移出--> given_up / removed
const (
	WaitlistStatusWaiting   = "waiting"   // 候补中
	WaitlistStatusNotified  = "notified"  // 已通知（管理员已电话联系）
	WaitlistStatusConverted = "converted" // 已转报名（用户完成正常报名支付）
	WaitlistStatusGivenUp   = "given_up"  // 已放弃（用户自助取消）
	WaitlistStatusRemoved   = "removed"   // 已移除（管理员移出队列）
)

// WaitlistActiveStatuses 是仍在队列中的状态集合（通知顺序的依据）。
var WaitlistActiveStatuses = []string{WaitlistStatusWaiting, WaitlistStatusNotified}

// 后台标记已通知时的通知结果枚举（API 文档 12.2 notify.note）。
const (
	WaitlistNoteConfirmedIntent = "confirmed_intent" // 有意向
	WaitlistNoteNoAnswer        = "no_answer"        // 无法接通
	WaitlistNoteCallbackLater   = "callback_later"   // 待回复
)

// Waitlist 是候补队列记录（waitlists 表）。证件号只存 HMAC 哈希与脱敏值，
// id_card_masked 供后台列表展示（无证件号登记为 NULL）。
type Waitlist struct {
	ID           int64
	EventID      int64
	GroupID      int64
	UserID       *int64
	DanceType    string
	Name         string
	IDCardHash   *string
	IDCardMasked *string
	Institution  *string
	Phone        string
	Status       string
	PositionNo   int
	NotifiedAt   *string
	HandledAt    *string
	CreatedAt    string
	UpdatedAt    string
}

// SubmitWaitlistRequest 是提交候补（POST /api/v1/events/{event_id}/waitlists，
// API 文档 5.1）的请求体。id_card_no 可选：无证件号的极端情况只登记姓名与电话
// （R7），有值时仅用于校验与哈希，服务端不落明文。
type SubmitWaitlistRequest struct {
	GroupID     int64  `json:"group_id" binding:"required"`
	Name        string `json:"name" binding:"required,max=30"`
	IDCardNo    string `json:"id_card_no"`
	Institution string `json:"institution" binding:"omitempty,max=50"`
	Phone       string `json:"phone" binding:"required"`
}

// SubmitWaitlistResponse 是提交候补的响应（API 文档 5.1）。
type SubmitWaitlistResponse struct {
	WaitlistID int64  `json:"waitlist_id"`
	PositionNo int    `json:"position_no"`
	Status     string `json:"status"`
}

// MyWaitlistItem 是「我的-我的候补」单条记录（API 文档 5.2）。
type MyWaitlistItem struct {
	WaitlistID int64  `json:"waitlist_id"`
	EventID    int64  `json:"event_id"`
	EventName  string `json:"event_name"`
	Level      string `json:"level"`
	DanceType  string `json:"dance_type"`
	PositionNo int    `json:"position_no"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

// MyWaitlistListResponse 是「我的候补」分页响应。
type MyWaitlistListResponse struct {
	List     []MyWaitlistItem `json:"list"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

// AdminWaitlistListQuery 是后台候补队列列表的筛选与分页条件（API 文档 12.1）。
type AdminWaitlistListQuery struct {
	EventID   int64
	Level     string
	DanceType string
	Status    string
	Page      int
	PageSize  int
}

// AdminWaitlistItem 是后台候补队列列表项（API 文档 12.1）。
// phone 为明文（仅本接口与导出使用）；id_card_masked 为脱敏证件号，无证件号登记为 null。
type AdminWaitlistItem struct {
	WaitlistID   int64   `json:"waitlist_id"`
	PositionNo   int     `json:"position_no"`
	Name         string  `json:"name"`
	IDCardMasked *string `json:"id_card_masked"`
	Institution  *string `json:"institution"`
	Phone        string  `json:"phone"`
	Status       string  `json:"status"`
	EventID      int64   `json:"event_id"`
	EventName    string  `json:"event_name"`
	GroupID      int64   `json:"group_id"`
	Level        string  `json:"level"`
	DanceType    string  `json:"dance_type"`
	CreatedAt    string  `json:"created_at"`
	NotifiedAt   *string `json:"notified_at"`
	HandledAt    *string `json:"handled_at"`
}

// AdminWaitlistListResponse 是后台候补队列分页响应（API 文档 12.1）。
// ReleasedQuota 是筛选范围内报名单元当前可用名额合计：仅当筛选定位到单个
// 报名单元（event_id + level + dance_type）时用于队列顶部"已释放 N 个名额"提示。
type AdminWaitlistListResponse struct {
	List          []AdminWaitlistItem `json:"list"`
	Total         int64               `json:"total"`
	Page          int                 `json:"page"`
	PageSize      int                 `json:"page_size"`
	ReleasedQuota int                 `json:"released_quota"`
}

// AdminWaitlistNotifyRequest 是标记已通知的请求体（API 文档 12.2）。
// Note 为通知结果枚举：confirmed_intent / no_answer / callback_later。
type AdminWaitlistNotifyRequest struct {
	Note string `json:"note" binding:"required"`
}

// AdminWaitlistRemoveRequest 是移出队列的请求体（API 文档 12.2）。
type AdminWaitlistRemoveRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// AdminWaitlistConvertRequest 是手动关联已支付订单的请求体（API 文档 12.2）。
type AdminWaitlistConvertRequest struct {
	OrderNo string `json:"order_no" binding:"required"`
}

// AdminWaitlistActionResponse 是候补队列操作的响应（API 文档 12.2）。
type AdminWaitlistActionResponse struct {
	WaitlistID int64  `json:"waitlist_id"`
	Status     string `json:"status"`
}
