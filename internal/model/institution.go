package model

type Institution struct {
	Name   string `json:"name"`
	Points int    `json:"points"`
}

type InstitutionRanking struct {
	Institutions []Institution `json:"institutions"`
}
