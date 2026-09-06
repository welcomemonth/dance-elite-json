package service

import (
	"errors"
	"testing"

	"github.com/meetmonth/dance-elite/internal/model"
)

// fakeStore 实现 Store 接口，用于隔离真实文件系统。
type fakeStore struct {
	cfg *model.Config
	err error
}

func (f fakeStore) GetConfig() (*model.Config, error) { return f.cfg, f.err }

func TestGetConfig(t *testing.T) {
	want := &model.Config{Season: "2026"}
	svc := NewService(fakeStore{cfg: want})

	got, err := svc.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig 失败: %v", err)
	}
	if got.Season != "2026" {
		t.Errorf("Season = %q, want 2026", got.Season)
	}
}

func TestGetConfig_Error(t *testing.T) {
	wantErr := errors.New("boom")
	svc := NewService(fakeStore{err: wantErr})

	if _, err := svc.GetConfig(); !errors.Is(err, wantErr) {
		t.Errorf("期望透传 %v, got %v", wantErr, err)
	}
}
