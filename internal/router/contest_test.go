package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/audio"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/service"
)

// newContestEngine 构造一个带真实音频目录的引擎，供 contest 路由测试使用。
func newContestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "U13-A9-刘芊惠-《白瓷旋香》.mp3"), []byte("dummy"), 0o644); err != nil {
		t.Fatalf("写入音频文件失败: %v", err)
	}
	m, err := audio.NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager 失败: %v", err)
	}
	return New(service.NewService(fakeStore{}), m)
}

func TestContestAudios(t *testing.T) {
	engine := newContestEngine(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/contest/audios", nil)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp model.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("code = %d, want 0", resp.Code)
	}
	data, ok := resp.Data.([]interface{})
	if !ok || len(data) != 1 {
		t.Fatalf("data = %#v, want 1 个音频", resp.Data)
	}
}

func TestContestAudio(t *testing.T) {
	engine := newContestEngine(t)

	// 用转义后的 URL 请求中文文件名，模拟浏览器真实行为
	p := "/api/contest/audio/" + url.PathEscape("U13-A9-刘芊惠-《白瓷旋香》.mp3")
	req := httptest.NewRequest(http.MethodGet, p, nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestContestAudio_HEAD(t *testing.T) {
	engine := newContestEngine(t)

	req := httptest.NewRequest(http.MethodHead, "/api/contest/audio/U13-A9-刘芊惠-《白瓷旋香》.mp3", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// HEAD 用于前端探测文件是否存在，应返回 200 且不写 body
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("HEAD 响应 body 应为空, got %d 字节", w.Body.Len())
	}
}

func TestContestAudio_NotFound(t *testing.T) {
	engine := newContestEngine(t)

	req := httptest.NewRequest(http.MethodGet, "/api/contest/audio/nope.mp3", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestContestAudio_PathTraversal(t *testing.T) {
	engine := newContestEngine(t)

	req := httptest.NewRequest(http.MethodGet, "/api/contest/audio/..%2F..%2Fconfig.json", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestContestAudio_Range(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.mp3"), []byte("0123456789"), 0o644); err != nil {
		t.Fatalf("写入音频文件失败: %v", err)
	}
	m, err := audio.NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager 失败: %v", err)
	}
	engine := New(service.NewService(fakeStore{}), m)

	req := httptest.NewRequest(http.MethodGet, "/api/contest/audio/a.mp3", nil)
	req.Header.Set("Range", "bytes=0-3")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", w.Code)
	}
	if cr := w.Header().Get("Content-Range"); cr != "bytes 0-3/10" {
		t.Errorf("Content-Range = %q, want %q", cr, "bytes 0-3/10")
	}
	if got := w.Body.String(); got != "0123" {
		t.Errorf("body = %q, want %q", got, "0123")
	}
}

func TestContestPage(t *testing.T) {
	engine := newContestEngine(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/contest", nil)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<!DOCTYPE html>") {
		t.Error("页面应包含 HTML 结构")
	}
}
