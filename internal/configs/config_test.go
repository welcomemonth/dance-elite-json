package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// clearEnv 清空指定环境变量，测试结束后恢复原值。
func clearEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		old, existed := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		if existed {
			t.Cleanup(func() { os.Setenv(key, old) })
		} else {
			t.Cleanup(func() { os.Unsetenv(key) })
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t, "PORT", "DEBUG", "DB_PATH", "PAY_MOCK", "ADMIN_INIT_USERNAME")

	cfg, err := read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.Debug {
		t.Error("Debug = true, want false")
	}
	if cfg.DBPath != "data/dance-elite.db" {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if cfg.PayMock {
		t.Error("PayMock = true, want false")
	}
	if cfg.AdminInitUsername != "admin" {
		t.Errorf("AdminInitUsername = %q, want admin", cfg.AdminInitUsername)
	}
}

func TestEnvOverridesDefaults(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("DEBUG", "true")
	t.Setenv("DB_PATH", "/tmp/dance.db")
	t.Setenv("PAY_MOCK", "1")
	t.Setenv("WECHAT_APPID", "wx_from_env")
	t.Setenv("ID_CARD_HMAC_KEY", "hmac_from_env")
	t.Setenv("ADMIN_INIT_PASSWORD", "admin_from_env")

	cfg, err := read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want env value", cfg.Port)
	}
	if !cfg.Debug {
		t.Error("Debug = false, want true")
	}
	if cfg.DBPath != "/tmp/dance.db" {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if !cfg.PayMock {
		t.Error("PayMock = false, want true")
	}
	if cfg.WechatAppID != "wx_from_env" {
		t.Errorf("WechatAppID = %q", cfg.WechatAppID)
	}
	if cfg.IDCardHMACKey != "hmac_from_env" {
		t.Errorf("IDCardHMACKey = %q", cfg.IDCardHMACKey)
	}
	if cfg.AdminInitPassword != "admin_from_env" {
		t.Errorf("AdminInitPassword = %q", cfg.AdminInitPassword)
	}
}

func TestInvalidBoolEnvReturnsError(t *testing.T) {
	t.Setenv("PAY_MOCK", "yes-please")

	_, err := read()
	if err == nil || !strings.Contains(err.Error(), "PAY_MOCK") {
		t.Fatalf("read error = %v, want invalid PAY_MOCK error", err)
	}
}

// TestLoadFailsFastOnMissingSecrets 验证启动自检在缺项时返回错误并列出全部缺失项。
func TestLoadFailsFastOnMissingSecrets(t *testing.T) {
	required := []string{"JWT_SECRET", "ID_CARD_HMAC_KEY", "WECHAT_APPID", "WECHAT_SECRET", "ADMIN_INIT_PASSWORD"}
	clearEnv(t, required...)
	t.Setenv("PAY_MOCK", "true")

	_, err := Load()
	if err == nil {
		t.Fatal("Load should fail without required secrets")
	}
	for _, want := range required {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not list %s", err, want)
		}
	}
}

// TestLoadReadsDotEnv 验证本地 .env 会被读取，且已存在的环境变量优先。
func TestLoadReadsDotEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	body := "PORT=8081\nJWT_SECRET=jwt\nID_CARD_HMAC_KEY=hmac\n" +
		"WECHAT_APPID=wx\nWECHAT_SECRET=secret\nADMIN_INIT_PASSWORD=pass\nPAY_MOCK=true\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	clearEnv(t, "PORT", "JWT_SECRET", "ID_CARD_HMAC_KEY", "WECHAT_APPID", "WECHAT_SECRET", "ADMIN_INIT_PASSWORD", "PAY_MOCK")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "8081" {
		t.Errorf("Port = %q, want value from .env", cfg.Port)
	}
	if cfg.JWTSecret != "jwt" {
		t.Errorf("JWTSecret = %q, want value from .env", cfg.JWTSecret)
	}
}

func TestRealEnvWinsOverDotEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	body := "PORT=8081\nJWT_SECRET=jwt\nID_CARD_HMAC_KEY=hmac\n" +
		"WECHAT_APPID=wx\nWECHAT_SECRET=secret\nADMIN_INIT_PASSWORD=pass\nPAY_MOCK=true\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	clearEnv(t, "PORT", "JWT_SECRET", "ID_CARD_HMAC_KEY", "WECHAT_APPID", "WECHAT_SECRET", "ADMIN_INIT_PASSWORD", "PAY_MOCK")
	t.Setenv("PORT", "9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want existing env to win", cfg.Port)
	}
}

func TestMissingSecretsOrder(t *testing.T) {
	got := (&Config{PayMock: true}).MissingSecrets()
	want := []string{"JWT_SECRET", "ID_CARD_HMAC_KEY", "WECHAT_APPID", "WECHAT_SECRET", "ADMIN_INIT_PASSWORD"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MissingSecrets = %v, want %v", got, want)
	}
}

func TestMissingSecretsAllPresent(t *testing.T) {
	cfg := &Config{
		JWTSecret:         "jwt",
		IDCardHMACKey:     "hmac",
		WechatAppID:       "wx",
		WechatSecret:      "secret",
		PayMock:           true,
		AdminInitPassword: "pass",
	}
	if got := cfg.MissingSecrets(); len(got) != 0 {
		t.Fatalf("MissingSecrets = %v, want none", got)
	}
}

func TestMissingSecretsRealPay(t *testing.T) {
	cfg := &Config{
		JWTSecret:         "jwt",
		IDCardHMACKey:     "hmac",
		WechatAppID:       "wx",
		WechatSecret:      "secret",
		PayMock:           false,
		AdminInitPassword: "pass",
	}
	got := cfg.MissingSecrets()
	if len(got) != 6 {
		t.Fatalf("MissingSecrets = %v, want 6 pay-related items", got)
	}
	for _, item := range got {
		if !strings.HasPrefix(item, "WECHAT_") {
			t.Errorf("unexpected missing item %q", item)
		}
	}
}
