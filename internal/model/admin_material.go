package model

// AdminMissingMaterialQuery 是资料催收名单筛选（API 文档 12.3）。
// 名单固定为 active 报名且 photo/audio/program_name 任一为空；
// MissingItem 可再按单项缺失过滤（photo/audio/program_name/contact_phone）。
// DeadlineState：open（未截止）/ closed（已截止只读）/ 空（不限）。
type AdminMissingMaterialQuery struct {
	EventID       int64
	Level         string
	DanceType     string
	MissingItem   string
	DeadlineState string
	Page          int
	PageSize      int
}

// AdminMissingMaterialItem 是催收名单单条记录。
type AdminMissingMaterialItem struct {
	RegistrationID    int64    `json:"registration_id"`
	OrderNo           *string  `json:"order_no"`
	EventID           int64    `json:"event_id"`
	EventName         string   `json:"event_name"`
	GroupID           int64    `json:"group_id"`
	Level             string   `json:"level"`
	DanceType         string   `json:"dance_type"`
	PlayerNo          string   `json:"player_no"`
	PlayerName        string   `json:"player_name"`
	ContactPhone      *string  `json:"contact_phone"`
	MaterialCompleted int      `json:"completed"`
	MaterialTotal     int      `json:"total"`
	MissingItems      []string `json:"missing_items"`
	MaterialDeadline  *string  `json:"material_deadline"`
	MaterialUpdatedAt *string  `json:"material_updated_at"`
	Contacted         bool     `json:"contacted"`
	ContactedAt       *string  `json:"contacted_at"`
}

// AdminMissingMaterialListResponse 是催收名单响应（统一分页结构）。
type AdminMissingMaterialListResponse struct {
	List     []AdminMissingMaterialItem `json:"list"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
}

// MarkMaterialContactedRequest 是标记已联系的请求体（note 可选）。
type MarkMaterialContactedRequest struct {
	Note string `json:"note" binding:"omitempty,max=200"`
}

// MarkMaterialContactedResponse 是标记已联系响应。
type MarkMaterialContactedResponse struct {
	RegistrationID int64 `json:"registration_id"`
	Contacted      bool  `json:"contacted"`
}
