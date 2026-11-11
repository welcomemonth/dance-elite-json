package model

// 小程序端赛事只读接口的请求/响应 DTO（API 文档 2.1/2.2，T7）。
// 与后台 AdminEventItem 不同：只暴露展示所需字段，不暴露内部字段
// （score_rules/prize_rules JSON、audio_review_token、season_id 等）。

// MiniappEventStatus 是小程序端展示状态（由赛事状态 + 时间线推导）。
const (
	MiniappStatusUpcoming    = "upcoming"    // 未开始报名（当前时间 < reg_start_at）
	MiniappStatusRegistering = "registering" // 报名中
	MiniappStatusRegClosed   = "reg_closed"  // 报名已截止
	MiniappStatusFinished    = "finished"    // 赛事已结束
)

// MiniappEventListQuery 是 GET /api/v1/events 的查询参数（API 文档 2.1）。
type MiniappEventListQuery struct {
	Status   string // upcoming/registering/reg_closed/finished，空 = 全部
	Page     int
	PageSize int
}

// MiniappGroupSummary 是赛事列表中的组别报名热度项（API 文档 2.1）。
type MiniappGroupSummary struct {
	Level      string `json:"level"`
	DanceType  string `json:"dance_type"`
	Registered int    `json:"registered"`
	Quota      int    `json:"quota"`
}

// MiniappEventItem 是小程序赛事列表项（API 文档 2.1）。
type MiniappEventItem struct {
	ID             int64                 `json:"id"`
	Name           string                `json:"name"`
	Level          string                `json:"level"`
	City           *string               `json:"city"`
	Venue          *string               `json:"venue"`
	EventDate      *string               `json:"event_date"`
	CoverURL       *string               `json:"cover_url"`
	Fee            int64                 `json:"fee"`
	Status         string                `json:"status"`
	RegEndAt       *string               `json:"reg_end_at"`
	GroupSummary   []MiniappGroupSummary `json:"group_summary"`
	RegisteredByMe bool                  `json:"registered_by_me"`
}

// MiniappEventListResponse 是赛事列表分页响应。
type MiniappEventListResponse struct {
	List     []MiniappEventItem `json:"list"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}

// MiniappTimeline 是详情中的时间线五节点（API 文档 2.2）。
type MiniappTimeline struct {
	RegStartAt      *string `json:"reg_start_at"`
	RegEndAt        *string `json:"reg_end_at"`
	LotteryAt       *string `json:"lottery_at"`
	MaterialCheckAt *string `json:"material_check_at"`
	StartAt         *string `json:"start_at"`
}

// MiniappEventGroup 是详情中的报名单元（API 文档 2.2）。
// Remaining = Quota - Taken，已取消/退款报名不占名额（taken 即时释放）。
type MiniappEventGroup struct {
	GroupID       int64  `json:"group_id"`
	Level         string `json:"level"`
	DanceType     string `json:"dance_type"`
	BirthYearFrom int    `json:"birth_year_from"`
	BirthYearTo   int    `json:"birth_year_to"`
	Quota         int    `json:"quota"`
	Registered    int    `json:"registered"`
	Remaining     int    `json:"remaining"`
	Enabled       bool   `json:"enabled"`
}

// MiniappEventDetail 是小程序赛事详情（API 文档 2.2）。
type MiniappEventDetail struct {
	ID                int64               `json:"id"`
	Name              string              `json:"name"`
	Level             string              `json:"level"`
	City              *string             `json:"city"`
	Venue             *string             `json:"venue"`
	EventDate         *string             `json:"event_date"`
	CoverURL          *string             `json:"cover_url"`
	Fee               int64               `json:"fee"`
	Status            string              `json:"status"`
	Timeline          MiniappTimeline     `json:"timeline"`
	Groups            []MiniappEventGroup `json:"groups"`
	RefundPolicy      *string             `json:"refund_policy"`
	WorkDurationLimit *string             `json:"work_duration_limit"`
	ProcessDesc       *string             `json:"process_desc"`
	HonorDesc         *string             `json:"honor_desc"`
}
