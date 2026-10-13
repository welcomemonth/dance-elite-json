package model

// ========= 积分榜结构体 ============

type RankingData struct {
	Version   string  `json:"version"`
	UpdatedAt string  `json:"updatedAt"`
	Boards    []Board `json:"boards"`
}

type Board struct {
	Category  string          `json:"category"`  // "U11-古典舞"
	Level     string          `json:"level"`     // "U11"
	DanceType string          `json:"danceType"` // "古典舞"
	Direct    []*DirectDancer `json:"direct"`
	Ranking   []*RankDancer   `json:"ranking"`
}

type DirectDancer struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Avatar string  `json:"avatar"`
	IdCard string  `json:"idCard,omitempty"`
	Awards []Award `json:"awards,omitempty"`
}

type RankDancer struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Avatar     string  `json:"avatar"`
	IdCard     string  `json:"idCard,omitempty"`
	Rank       int     `json:"rank"`
	PrevRank   int     `json:"prevRank"`   // 0 表示上期不存在
	RankChange int     `json:"rankChange"` // 正数上升，负数下降
	Points     int     `json:"points"`
	Awards     []Award `json:"awards,omitempty"`
}

type RegularScores struct {
	A     float64 `json:"A"`     // 评委A
	B     float64 `json:"B"`     // 评委B
	C     float64 `json:"C"`     // 评委C
	Total float64 `json:"total"` // 常规赛总得分
}
