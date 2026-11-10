package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/meetmonth/dance-elite/internal/pkg/logger"
	"github.com/spf13/viper"
)

// ServerConfig 服务器配置
type ServerConfig struct {
	Port  string `mapstructure:"port"`
	Debug bool   `mapstructure:"debug"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	// Path 是 SQLite 数据库文件路径，如 data/dance-elite.db
	Path string `mapstructure:"path"`
}

// JWTConfig JWT 配置
type JWTConfig struct {
	Secret string `mapstructure:"secret"`
}

// WechatConfig 微信小程序及支付配置
type WechatConfig struct {
	AppID             string `mapstructure:"app_id"`
	Secret            string `mapstructure:"secret"`
	MchID             string `mapstructure:"mch_id"`
	MchAPIKey         string `mapstructure:"mch_api_key"`
	MchSerialNo       string `mapstructure:"mch_serial_no"`
	MchPrivateKeyPath string `mapstructure:"mch_private_key_path"`
	NotifyURL         string `mapstructure:"notify_url"`
}

// PayConfig 微信支付配置（T3 填值；T10 接入真实/ mock 通道）。
// 敏感字段（密钥等）在 T3 改为环境变量注入，yaml 只放非敏感项。
type PayConfig struct {
	// Mock 为 true 时走 fake 支付通道，无真实商户号也可联调（T10）
	Mock bool `mapstructure:"mock"`
}

// Config 全局配置
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Wechat   WechatConfig   `mapstructure:"wechat"`
	Pay      PayConfig      `mapstructure:"pay"`
}

// LoadConfig 从配置文件和环境变量加载配置
func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exeDir := filepath.Dir(exePath)
	viper.AddConfigPath(filepath.Join(exeDir, "configs"))
	viper.AddConfigPath("configs")
	viper.AddConfigPath("/opt/danceElite/configs")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	err = logger.Init(cfg.Server.Debug) // 日志组件初始化
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
