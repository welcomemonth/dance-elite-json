package model

type Dancer struct {
	ID       string  `json:"id"`
	IdCard   string  `json:"idCard"`
	Name     string  `json:"name"`
	Avatar   string  `json:"avatar"`
	IsDirect bool    `json:"isDirect"`
	Awards   []Award `json:"awards"`
}
