package model

// 配置
type Config struct {
	Season       string   `json:"season"`
	Title        string   `json:"title"`
	Subtitle     string   `json:"subtitle"`
	Levels       []string `json:"levels"`
	DanceStyles  []string `json:"danceStyles"`
	AdvanceQuota int      `json:"advanceQuota"`
	UpdatedAt    string   `json:"updatedAt"`
}
