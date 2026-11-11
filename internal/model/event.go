package model

// 赛季状态枚举（seasons.status）。
const (
	SeasonStatusUpcoming = "upcoming"
	SeasonStatusActive   = "active"
	SeasonStatusArchived = "archived"
)

// EventStatus 是赛事状态枚举（events.status，API 文档 10.1）。
// 状态机：draft → unpublished → registering → reg_closed → ongoing → finished；
// offline 为任意非终态可进入的终止态（下架，API 文档 10.4）。
const (
	EventStatusDraft       = "draft"
	EventStatusUnpublished = "unpublished"
	EventStatusRegistering = "registering"
	EventStatusRegClosed   = "reg_closed"
	EventStatusOngoing     = "ongoing"
	EventStatusFinished    = "finished"
	EventStatusOffline     = "offline"
)

// EventLevel 是赛事级别枚举（events.level）。
const (
	EventLevelGradeA = "grade_a" // 甲级赛
	EventLevelSuper  = "super"   // 超级赛
	EventLevelFinal  = "final"   // 年度总决赛
)

// GroupLevel 是组别年龄组枚举（event_groups.level）。
const (
	GroupLevelU11 = "U11"
	GroupLevelU13 = "U13"
	GroupLevelU15 = "U15"
)

// DanceType 是舞种枚举（event_groups.dance_type）。
const (
	DanceTypeClassical = "classical" // 古典舞
	DanceTypeFolk      = "folk"      // 民族民间舞
)

// Season 是赛季（seasons 表）。只做时间范围与归档，不挂规则；
// group_rules 为系统级组别年龄规则 JSON，创建赛事组别时从此复制。
type Season struct {
	ID         int64
	Name       string
	StartDate  *string
	EndDate    *string
	Status     string
	GroupRules string
	ArchivedAt *string
	CreatedAt  string
	UpdatedAt  string
}

// AdminSeasonRequest 是创建/编辑赛季（POST/PUT /api/v1/admin/seasons）的请求体。
// 创建时 status 忽略（恒为 upcoming，group_rules 从系统设置复制）；
// 编辑时全部字段可选，status 只允许 upcoming → active → archived 顺序流转。
type AdminSeasonRequest struct {
	Name      string `json:"name" binding:"required"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Status    string `json:"status"`
}

// Event 是赛事（events 表）。score_rules / prize_rules 为 JSON 文本。
type Event struct {
	ID                int64
	SeasonID          int64
	Name              string
	Level             string
	City              *string
	Venue             *string
	EventDate         *string
	Fee               int64
	CoverPath         *string
	RegStartAt        *string
	RegEndAt          *string
	LotteryAt         *string
	MaterialCheckAt   *string
	StartAt           *string
	Status            string
	ScoreRules        *string
	PrizeRules        *string
	RefundPolicy      *string
	WorkDurationLimit *string
	ProcessDesc       *string
	HonorDesc         *string
	ExcludeFromPrize  bool
	AudioReviewToken  *string
	CreatedAt         string
	UpdatedAt         string
}

// EventGroup 是赛事组别×舞种配置（event_groups 表）。一行 = 一个可报名单元。
type EventGroup struct {
	ID            int64
	EventID       int64
	Level         string
	DanceType     string
	BirthYearFrom int
	BirthYearTo   int
	Quota         int
	Taken         int
	Enabled       bool
	CreatedAt     string
	UpdatedAt     string
}
