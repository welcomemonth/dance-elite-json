package model

// 奖金
type Award struct {
	Event         string         `json:"event"`                   // 哪场赛事
	Name          string         `json:"name"`                    // 奖项名称
	Prize         string         `json:"prize"`                   // 奖金
	Group         string         `json:"group"`                   // 组别
	Point         string         `json:"point"`                   // 积分
	RegularScores *RegularScores `json:"regularScores,omitempty"` // 常规赛评委评分
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
