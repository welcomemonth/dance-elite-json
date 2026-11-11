package model

// 选手性别枚举（players.gender），由身份证第 17 位顺序码解析或后台录入时填写。
const (
	GenderMale   = "male"
	GenderFemale = "female"
)

// Player 是选手档案（players 表）。
//
// 选手唯一标识是系统生成的 player_no（8 位字母数字）；证件号只存
// id_card_hash（HMAC-SHA256，服务端密钥），判重/匹配按哈希重算比对。
// 档案与用户（users）之间没有直接外键，通过报名单（registrations）关联。
type Player struct {
	ID          int64
	PlayerNo    string
	Name        string
	IDCardHash  *string
	IDType      string
	Gender      *string
	BirthDate   *string // YYYY-MM-DD，组别年龄判定的唯一依据（与证件类型解耦）
	Nationality *string // 国籍/地区（后台人工录入可填，T16）
	AvatarPath  *string
	MergedInto  *int64 // 重复档案合并指向（T16 档案合并功能使用）
	CreatedAt   string
	UpdatedAt   string
}

// MergePlayerRequest 是后台档案合并请求（T16）：把路径中的重复档案合并到
// target_player_id 指向的保留档案。
type MergePlayerRequest struct {
	TargetPlayerID int64 `json:"target_player_id" binding:"required"`
}

// MergePlayerResponse 是档案合并响应：player_id 为被合并档案，
// merged_into 为保留档案，moved 为各业务表迁移行数。
type MergePlayerResponse struct {
	PlayerID   int64            `json:"player_id"`
	MergedInto int64            `json:"merged_into"`
	Moved      map[string]int64 `json:"moved"`
}
