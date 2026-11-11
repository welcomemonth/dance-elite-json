package model

// 人工报名收费方式（API 文档 11.5 charge_type）。
const (
	ManualChargeOfflinePaid = "offline_paid" // 线下已收款（金额必填 > 0）
	ManualChargeFree        = "free"         // 免费（总决赛、赠票等，金额固定 0）
)

// ManualRegistrationRequest 是后台人工报名请求体（API 文档 11.5，T16）。
// 用于无中国大陆身份证号选手：证件号与出生日期人工填写，证件号哈希建档；
// 组别由管理员手选（同一套年龄规则校验），订单直接置已支付（线下收款）。
type ManualRegistrationRequest struct {
	EventID      int64  `json:"event_id" binding:"required"`
	GroupID      int64  `json:"group_id" binding:"required"`
	PlayerName   string `json:"player_name" binding:"required,max=50"`
	IDType       string `json:"id_type"`
	IDCardNo     string `json:"id_card_no" binding:"required"`
	BirthDate    string `json:"birth_date"`
	Gender       string `json:"gender" binding:"omitempty,oneof=male female"`
	Nationality  string `json:"nationality" binding:"omitempty,max=50"`
	Institution  string `json:"institution" binding:"omitempty,max=50"`
	ContactPhone string `json:"contact_phone" binding:"required"`
	ChargeType   string `json:"charge_type" binding:"required"`
	Amount       int64  `json:"amount"`
	CSTicketID   int64  `json:"cs_ticket_id"`
	Remark       string `json:"remark" binding:"omitempty,max=500"`
}

// ManualRegistrationResponse 是人工报名响应（API 文档 11.5）。
type ManualRegistrationResponse struct {
	RegistrationID int64  `json:"registration_id"`
	OrderNo        string `json:"order_no"`
	PlayerNo       string `json:"player_no"`
}
