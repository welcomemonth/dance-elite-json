package model

// 奖金
type Award struct {
	Event string `json:"event"`
	Prize int    `json:"prize"`
}

type LeaderboardDancer struct {
	DancerID     int64   `json:"dancerId"`
	Name         string  `json:"name"`
	Avatar       string  `json:"avatar"`
	TotalScore   int     `json:"totalScore"`
	Rank         int     `json:"rank"`
	RankChange   int     `json:"rankChange"`
	IsDirect     bool    `json:"isDirect"`
	DirectReason string  `json:"directReason,omitempty"`
	Awards       []Award `json:"awards"`
}

// 积分榜响应
type LeaderboardData struct {
	Level        string              `json:"level"`
	DanceStyle   string              `json:"danceStyle"`
	UpdatedAt    string              `json:"updatedAt"`
	AdvanceQuota int                 `json:"advanceQuota"`
	Direct       []LeaderboardDancer `json:"direct"`
	Rankings     []LeaderboardDancer `json:"rankings"`
}
