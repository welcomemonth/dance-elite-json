// Package config 负责在启动时确定全部运行配置。
//
// 配置全部来自环境变量：本地开发由 godotenv 读取 .env（可选，已存在的
// 环境变量优先），生产由容器运行时注入。不存在 YAML 配置文件，密钥永远
// 不落文件入库；必需项缺失时启动直接失败。
package config

// Config 是全部运行配置。字段名与环境变量一一对应（见 load.go）。
type Config struct {
	// 服务
	Port    string
	Debug   bool
	DBPath  string
	PayMock bool

	// 鉴权与安全
	JWTSecret        string
	JWTExpireSeconds int
	IDCardHMACKey    string

	// 微信小程序
	WechatAppID  string
	WechatSecret string

	// 微信支付
	WechatMchID       string
	WechatPayKey      string
	WechatMchSerialNo string
	WechatPayCert     string
	WechatPrivateKey  string
	WechatNotifyURL   string

	// 后台初始超管（仅首次启动建档，已存在账号不覆盖）
	AdminInitUsername string
	AdminInitPassword string
}

// requirement 是一个必需配置项及其当前值。
type requirement struct {
	env   string
	value string
}

// MissingSecrets 返回缺失的必需环境变量名（按固定顺序，便于日志阅读）。
// 返回空切片表示自检通过。
func (c *Config) MissingSecrets() []string {
	required := []requirement{
		{"JWT_SECRET", c.JWTSecret},
		{"ID_CARD_HMAC_KEY", c.IDCardHMACKey},
		{"WECHAT_APPID", c.WechatAppID},
		{"WECHAT_SECRET", c.WechatSecret},
		{"ADMIN_INIT_PASSWORD", c.AdminInitPassword},
	}
	if !c.PayMock {
		required = append(required,
			requirement{"WECHAT_MCHID", c.WechatMchID},
			requirement{"WECHAT_PAY_KEY", c.WechatPayKey},
			requirement{"WECHAT_MCH_SERIAL_NO", c.WechatMchSerialNo},
			requirement{"WECHAT_PAY_CERT", c.WechatPayCert},
			requirement{"WECHAT_PAY_PRIVATE_KEY", c.WechatPrivateKey},
			requirement{"WECHAT_PAY_NOTIFY_URL", c.WechatNotifyURL},
		)
	}

	var missing []string
	for _, item := range required {
		if item.value == "" {
			missing = append(missing, item.env)
		}
	}
	return missing
}
