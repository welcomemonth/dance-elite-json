package model

type DirectRecord struct {
	Name   string  `json:"name"`   // 选手名字
	Point  string  `json:"point"`  // 选手积分
	Avatar string  `json:"avatar"` // 选手头像
	Awards []Award `json:"award"`  // 选手获得的奖项
}

type PointRecord struct {
	Name       string `json:"name"`       // 选手名字
	Point      string `json:"point"`      // 选手积分
	Avatar     string `json:"avatar"`     // 选手头像
	Rank       int    `json:"rank"`       // 真实排名
	ChangeRank string `json:"changeRank"` //排名变化
}

type RankGroup struct {
	Category string    `json:"category"` // 例如: "U11-古典舞"
	Dancers  []*Dancer `json:"dancers"`  // 已按积分降序排列的选手列表
}

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
