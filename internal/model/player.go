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
	ID         int64
	PlayerNo   string
	Name       string
	IDCardHash *string
	IDType     string
	Gender     *string
	BirthDate  *string // YYYY-MM-DD，组别年龄判定的唯一依据（与证件类型解耦）
	AvatarPath *string
	MergedInto *int64 // 重复档案合并指向（T16 档案合并功能使用）
	CreatedAt  string
	UpdatedAt  string
}
