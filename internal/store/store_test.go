package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

func writeConfig(t *testing.T, dir string, cfg model.Config) {
	t.Helper()
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("序列化配置失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), b, 0o644); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}
}

func TestGetConfig(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	want := model.Config{Season: "2026", Title: "测试标题"}
	writeConfig(t, dir, want)

	got, err := NewStore(dir).GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig 失败: %v", err)
	}
	if got.Season != want.Season || got.Title != want.Title {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestGetConfig_NotFound(t *testing.T) {
	ctx := context.Background()
	// 目录下没有 config.json
	if _, err := NewStore(t.TempDir()).GetConfig(ctx); err == nil {
		t.Error("缺少 config.json 时应返回错误")
	}
}
