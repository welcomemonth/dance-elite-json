package model

// AdminRegistrationListQuery 是后台报名名单筛选（开发计划 T17）。
// Status 缺省为 active；MaterialState 取 complete/incomplete（空串不筛）。
// MissingItem 可指定单项缺失（photo/audio/program_name/contact_phone）。
type AdminRegistrationListQuery struct {
	EventID       int64
	GroupID       int64
	Level         string
	DanceType     string
	Status        string
	MaterialState string
	MissingItem   string
	Keyword       string // 选手姓名 / player_no / 订单号
	Page          int
	PageSize      int
}

// AdminRegistrationItem 是后台报名名单单条记录。
// contact_phone 取报名单填写值，缺省回退用户手机号；证件号与订单列表口径一致，
// 仅以 player_no + id_type 标识选手。
type AdminRegistrationItem struct {
	RegistrationID    int64    `json:"registration_id"`
	OrderNo           *string  `json:"order_no"`
	EventID           int64    `json:"event_id"`
	EventName         string   `json:"event_name"`
	GroupID           int64    `json:"group_id"`
	Level             string   `json:"level"`
	DanceType         string   `json:"dance_type"`
	PlayerNo          string   `json:"player_no"`
	PlayerName        string   `json:"player_name"`
	IDType            string   `json:"id_type"`
	Institution       *string  `json:"institution"`
	ContactPhone      *string  `json:"contact_phone"`
	Status            string   `json:"status"`
	Source            string   `json:"source"`
	MaterialCompleted int      `json:"material_completed"`
	MaterialTotal     int      `json:"material_total"`
	MissingItems      []string `json:"missing_items"`
	MaterialUpdatedAt *string  `json:"material_updated_at"`
	CreatedAt         string   `json:"created_at"`
}

// AdminRegistrationListResponse 是后台报名名单响应（统一分页结构）。
type AdminRegistrationListResponse struct {
	List     []AdminRegistrationItem `json:"list"`
	Total    int64                   `json:"total"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
}
