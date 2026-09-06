package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/service"
)

// fakeStore 实现 service.Store 接口，隔离真实文件系统。
type fakeStore struct {
	cfg *model.Config
	err error
}

func (f fakeStore) GetConfig() (*model.Config, error) { return f.cfg, f.err }

func TestPing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := New(service.NewService(fakeStore{}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := New(service.NewService(fakeStore{
		cfg: &model.Config{Season: "2026", Title: "国舞菁英"},
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Code    int          `json:"code"`
		Message string       `json:"message"`
		Data    model.Config `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("code = %d, want 0", resp.Code)
	}
	if resp.Data.Season != "2026" || resp.Data.Title != "国舞菁英" {
		t.Errorf("data = %+v", resp.Data)
	}
}

func TestConfig_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := New(service.NewService(fakeStore{err: errors.New("boom")}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	engine.ServeHTTP(w, req)

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Code != 1001 {
		t.Fatalf("code = %d, want 1001", resp.Code)
	}
}
