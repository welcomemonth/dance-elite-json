package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Load 在启动时一次性确定全部配置，并在必需项缺失时直接失败。
//
// 来源：代码默认值 → .env（本地开发，可选）→ 环境变量。
// godotenv 不会覆盖已存在的环境变量，因此生产/容器注入的值始终优先。
func Load() (*Config, error) {
	cfg, err := read()
	if err != nil {
		return nil, err
	}
	if missing := cfg.MissingSecrets(); len(missing) > 0 {
		return nil, fmt.Errorf("启动自检失败，缺少必需环境变量: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

// read 只负责加载 .env 与读取环境变量，不做必需项校验（便于测试）。
func read() (*Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("读取 .env: %w", err)
	}

	debug, err := envBool("DEBUG", false)
	if err != nil {
		return nil, err
	}
	payMock, err := envBool("PAY_MOCK", false)
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:    envOr("PORT", "8080"),
		Debug:   debug,
		DBPath:  envOr("DB_PATH", "data/dance-elite.db"),
		PayMock: payMock,

		JWTSecret:     os.Getenv("JWT_SECRET"),
		IDCardHMACKey: os.Getenv("ID_CARD_HMAC_KEY"),

		WechatAppID:  os.Getenv("WECHAT_APPID"),
		WechatSecret: os.Getenv("WECHAT_SECRET"),

		WechatMchID:       os.Getenv("WECHAT_MCHID"),
		WechatPayKey:      os.Getenv("WECHAT_PAY_KEY"),
		WechatMchSerialNo: os.Getenv("WECHAT_MCH_SERIAL_NO"),
		WechatPayCert:     os.Getenv("WECHAT_PAY_CERT"),
		WechatPrivateKey:  os.Getenv("WECHAT_PAY_PRIVATE_KEY"),
		WechatNotifyURL:   os.Getenv("WECHAT_PAY_NOTIFY_URL"),

		AdminInitUsername: envOr("ADMIN_INIT_USERNAME", "admin"),
		AdminInitPassword: os.Getenv("ADMIN_INIT_PASSWORD"),
	}, nil
}

// envOr 读取字符串环境变量，未设置或为空时返回默认值。
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envBool 读取布尔环境变量，未设置时返回默认值；非法值返回错误而非静默忽略。
func envBool(key string, def bool) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("环境变量 %s=%q 不是合法布尔值（可用 true/false/1/0）", key, raw)
	}
	return v, nil
}
