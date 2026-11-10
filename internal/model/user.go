// Package model 定义持久化实体与各端请求/响应 DTO。
//
// 字段命名遵循 API 约定（snake_case 由 JSON tag 表达）；金额单位为分，
// 时间字段为 ISO 8601 本地时间字符串（见 internal/utils.NowISO）。
package model

// UserStatus 是 users.status 的枚举值。
const (
	UserStatusActive  = "active"
	UserStatusBlocked = "blocked"
)

// User 是微信用户账号（users 表）。
//
// 选手档案（players）与用户之间没有直接外键，二者通过报名单
// （registrations.user_id / player_id）关联；登录响应中的 player_no
// 取自该用户最近一条报名对应的选手。
type User struct {
	ID          int64
	OpenID      string
	UnionID     *string
	Nickname    *string
	AvatarURL   *string
	Phone       *string
	Status      string
	LastLoginAt *string
	CreatedAt   string
	UpdatedAt   string
}
