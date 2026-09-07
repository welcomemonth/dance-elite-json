package model

type DirectRecord struct {
	Name   string  `json:"name"`   // 选手名字
	Point  string  `json:"point"`  // 选手积分
	Avatar string  `json:"avatar"` // 选手头像
	Awards []Award `json:"award"`  // 选手获得的奖项
}

type PointRecord struct {
	Name       string `json:"name"`   // 选手名字
	Point      string `json:"point"`  // 选手积分
	Avatar     string `json:"avatar"` // 选手头像
	Rank       int    `json:"rank"`   // 真实排名
	ChangeRank string `json:"-"`      //排名变化
}

type RankGroup struct {
	Category string    `json:"category"` // 例如: "U11-古典舞"
	Dancers  []*Dancer `json:"dancers"`  // 已按积分降序排列的选手列表
}
