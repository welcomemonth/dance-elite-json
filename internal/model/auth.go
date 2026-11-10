package model

// WxLoginRequest 是 POST /api/v1/auth/wx-login 的请求体。
type WxLoginRequest struct {
	Code      string `json:"code" binding:"required"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// UserInfo 是登录/用户信息响应中的用户视图。
// Phone 为脱敏手机号；PlayerNo 取最近一条报名关联的选手编号，无则为 null。
type UserInfo struct {
	ID        int64   `json:"id"`
	Nickname  *string `json:"nickname"`
	AvatarURL *string `json:"avatar_url"`
	Phone     *string `json:"phone"`
	PlayerNo  *string `json:"player_no"`
}

// WxLoginResponse 是微信登录成功后的响应数据。
type WxLoginResponse struct {
	Token     string   `json:"token"`
	ExpiresIn int      `json:"expires_in"`
	IsNew     bool     `json:"is_new"`
	User      UserInfo `json:"user"`
}

// BindPhoneRequest 是 PUT /api/v1/auth/phone 的请求体。
type BindPhoneRequest struct {
	PhoneCode string `json:"phone_code" binding:"required"`
}

// BindPhoneResponse 返回绑定后的脱敏手机号。
type BindPhoneResponse struct {
	Phone string `json:"phone"`
}
