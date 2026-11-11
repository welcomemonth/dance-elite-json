package model

// 后台赛事/赛季管理的请求与响应 DTO（API 文档 10.x，赛季接口见 10.7）。

// EventTimeline 是赛事时间线五节点（API 文档 10.2），强制先后：
// reg_start_at < reg_end_at < lottery_at < material_check_at < start_at。
type EventTimeline struct {
	RegStartAt      string `json:"reg_start_at"`
	RegEndAt        string `json:"reg_end_at"`
	LotteryAt       string `json:"lottery_at"`
	MaterialCheckAt string `json:"material_check_at"`
	StartAt         string `json:"start_at"`
}

// AdminEventGroupInput 是创建/编辑赛事时传入的一个报名单元（组别×舞种）。
// 不带 GroupID = 新增组合（仅草稿/未发布赛事允许）；带 GroupID = 调整已有组别名额。
type AdminEventGroupInput struct {
	GroupID   *int64 `json:"group_id"`
	Level     string `json:"level"`
	DanceType string `json:"dance_type"`
	Quota     int    `json:"quota"`
}

// AdminCreateEventRequest 是 POST /api/v1/admin/events 的请求体（API 文档 10.2）。
// ScoreRules / PrizeRules 为可选 JSON 文本，缺省用系统默认模板预填。
type AdminCreateEventRequest struct {
	Name              string                 `json:"name" binding:"required"`
	SeasonID          int64                  `json:"season_id" binding:"required"`
	Level             string                 `json:"level" binding:"required"`
	City              string                 `json:"city"`
	Venue             string                 `json:"venue" binding:"required"`
	EventDate         string                 `json:"event_date" binding:"required"`
	Fee               *int64                 `json:"fee"`
	Timeline          EventTimeline          `json:"timeline" binding:"required"`
	Groups            []AdminEventGroupInput `json:"groups" binding:"required,min=1"`
	ScoreRules        string                 `json:"score_rules"`
	PrizeRules        string                 `json:"prize_rules"`
	RefundPolicy      string                 `json:"refund_policy"`
	WorkDurationLimit string                 `json:"work_duration_limit"`
	ProcessDesc       string                 `json:"process_desc"`
	HonorDesc         string                 `json:"honor_desc"`
	ExcludeFromPrize  bool                   `json:"exclude_from_prize"`
	Publish           bool                   `json:"publish"`
}

// AdminUpdateEventRequest 是 PUT /api/v1/admin/events/{event_id} 的请求体（API 文档 10.3）。
// 已发布赛事：时间线、级别不可改；groups 只允许带 group_id 调大名额。
type AdminUpdateEventRequest struct {
	Name              string                 `json:"name" binding:"required"`
	Level             string                 `json:"level" binding:"required"`
	City              string                 `json:"city"`
	Venue             string                 `json:"venue" binding:"required"`
	EventDate         string                 `json:"event_date" binding:"required"`
	Fee               *int64                 `json:"fee"`
	Timeline          EventTimeline          `json:"timeline" binding:"required"`
	Groups            []AdminEventGroupInput `json:"groups" binding:"required,min=1"`
	ScoreRules        string                 `json:"score_rules"`
	PrizeRules        string                 `json:"prize_rules"`
	RefundPolicy      string                 `json:"refund_policy"`
	WorkDurationLimit string                 `json:"work_duration_limit"`
	ProcessDesc       string                 `json:"process_desc"`
	HonorDesc         string                 `json:"honor_desc"`
	ExcludeFromPrize  bool                   `json:"exclude_from_prize"`
}

// AdminEventMutationResponse 是赛事创建/编辑/状态流转的响应（API 文档 10.2/10.3/10.4）。
type AdminEventMutationResponse struct {
	EventID int64  `json:"event_id"`
	Status  string `json:"status"`
}

// AdminEventGroupProgress 是后台赛事列表/详情中的组别报名进度项（API 文档 10.1）。
type AdminEventGroupProgress struct {
	GroupID       int64  `json:"group_id"`
	Level         string `json:"level"`
	DanceType     string `json:"dance_type"`
	BirthYearFrom int    `json:"birth_year_from"`
	BirthYearTo   int    `json:"birth_year_to"`
	Quota         int    `json:"quota"`
	Registered    int    `json:"registered"`
	Enabled       bool   `json:"enabled"`
}

// AdminEventItem 是后台赛事列表项（API 文档 10.1）：赛事完整字段 + 组别报名进度。
// 使用显式响应 DTO（而非嵌入 model.Event）：接口字段一律 snake_case，
// 且避免内部字段新增时被动暴露。
type AdminEventItem struct {
	ID                int64                     `json:"id"`
	SeasonID          int64                     `json:"season_id"`
	Name              string                    `json:"name"`
	Level             string                    `json:"level"`
	City              *string                   `json:"city"`
	Venue             *string                   `json:"venue"`
	EventDate         *string                   `json:"event_date"`
	Fee               int64                     `json:"fee"`
	CoverPath         *string                   `json:"cover_path"`
	RegStartAt        *string                   `json:"reg_start_at"`
	RegEndAt          *string                   `json:"reg_end_at"`
	LotteryAt         *string                   `json:"lottery_at"`
	MaterialCheckAt   *string                   `json:"material_check_at"`
	StartAt           *string                   `json:"start_at"`
	Status            string                    `json:"status"`
	ScoreRules        *string                   `json:"score_rules"`
	PrizeRules        *string                   `json:"prize_rules"`
	RefundPolicy      *string                   `json:"refund_policy"`
	WorkDurationLimit *string                   `json:"work_duration_limit"`
	ProcessDesc       *string                   `json:"process_desc"`
	HonorDesc         *string                   `json:"honor_desc"`
	ExcludeFromPrize  bool                      `json:"exclude_from_prize"`
	AudioReviewToken  *string                   `json:"audio_review_token"`
	CreatedAt         string                    `json:"created_at"`
	UpdatedAt         string                    `json:"updated_at"`
	Groups            []AdminEventGroupProgress `json:"groups"`
}

// NewAdminEventItem 由赛事实体与组别进度构造后台列表项。
func NewAdminEventItem(e Event, groups []AdminEventGroupProgress) AdminEventItem {
	return AdminEventItem{
		ID:                e.ID,
		SeasonID:          e.SeasonID,
		Name:              e.Name,
		Level:             e.Level,
		City:              e.City,
		Venue:             e.Venue,
		EventDate:         e.EventDate,
		Fee:               e.Fee,
		CoverPath:         e.CoverPath,
		RegStartAt:        e.RegStartAt,
		RegEndAt:          e.RegEndAt,
		LotteryAt:         e.LotteryAt,
		MaterialCheckAt:   e.MaterialCheckAt,
		StartAt:           e.StartAt,
		Status:            e.Status,
		ScoreRules:        e.ScoreRules,
		PrizeRules:        e.PrizeRules,
		RefundPolicy:      e.RefundPolicy,
		WorkDurationLimit: e.WorkDurationLimit,
		ProcessDesc:       e.ProcessDesc,
		HonorDesc:         e.HonorDesc,
		ExcludeFromPrize:  e.ExcludeFromPrize,
		AudioReviewToken:  e.AudioReviewToken,
		CreatedAt:         e.CreatedAt,
		UpdatedAt:         e.UpdatedAt,
		Groups:            groups,
	}
}

// AdminSeasonItem 是后台赛季接口的响应对象（API 文档 10.7：完整赛季对象）。
type AdminSeasonItem struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	StartDate  *string `json:"start_date"`
	EndDate    *string `json:"end_date"`
	Status     string  `json:"status"`
	GroupRules string  `json:"group_rules"`
	ArchivedAt *string `json:"archived_at"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// NewAdminSeasonItem 由赛季实体构造后台响应对象。
func NewAdminSeasonItem(s Season) AdminSeasonItem {
	return AdminSeasonItem{
		ID:         s.ID,
		Name:       s.Name,
		StartDate:  s.StartDate,
		EndDate:    s.EndDate,
		Status:     s.Status,
		GroupRules: s.GroupRules,
		ArchivedAt: s.ArchivedAt,
		CreatedAt:  s.CreatedAt,
		UpdatedAt:  s.UpdatedAt,
	}
}

// AdminEventListResponse 是 GET /api/v1/admin/events 的分页响应。
type AdminEventListResponse struct {
	List     []AdminEventItem `json:"list"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

// AdminEventListQuery 是 GET /api/v1/admin/events 的查询参数（API 文档 10.1）。
type AdminEventListQuery struct {
	Status   string
	Level    string
	Keyword  string
	Page     int
	PageSize int
}

// AdminGroupQuotaRequest 是 PUT .../groups/{group_id}/quota 的请求体（API 文档 10.5）。
type AdminGroupQuotaRequest struct {
	Quota int `json:"quota" binding:"required"`
}

// AdminGroupQuotaResponse 是名额调整后的响应（API 文档 10.5）。
type AdminGroupQuotaResponse struct {
	GroupID    int `json:"group_id"`
	Quota      int `json:"quota"`
	Registered int `json:"registered"`
	Remaining  int `json:"remaining"`
}

// AdminGroupEnabledResponse 是关闭/重开组别后的响应（API 文档 10.5）。
type AdminGroupEnabledResponse struct {
	GroupID int  `json:"group_id"`
	Enabled bool `json:"enabled"`
}
