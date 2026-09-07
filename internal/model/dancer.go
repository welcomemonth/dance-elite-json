package model

type Dancer struct {
	ID       string  `json:"id"`
	IdCard   string  `json:"idCard"`
	Name     string  `json:"name"`
	IsDirect bool    `json:"isDirect"`
	Awards   []Award `json:"awards"`
}
