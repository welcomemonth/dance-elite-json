package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(HandleErrors())
	return e
}

// TestErrorMapping 每种 BizError 映射到正确的 HTTP Status 与业务错误码。
func TestErrorMapping(t *testing.T) {
	cases := []struct {
		name     string
		err      *BizError
		wantHTTP int
		wantCode int
	}{
		{"bad_request", ErrBadRequest("参数错"), http.StatusBadRequest, CodeBadRequest},
		{"unauthorized", ErrUnauthorized("未登录"), http.StatusUnauthorized, CodeUnauthorized},
		{"forbidden", ErrForbidden("无权限"), http.StatusForbidden, CodeForbidden},
		{"not_found", ErrNotFound("不存在"), http.StatusNotFound, CodeNotFound},
		{"conflict", ErrConflict("重复报名"), http.StatusConflict, CodeConflict},
		{"too_many", ErrTooManyRequests("限流"), http.StatusTooManyRequests, CodeTooManyRequests},
		{"internal", ErrInternal(errors.New("db down"), ""), http.StatusInternalServerError, CodeInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine()
			e.GET("/x", func(c *gin.Context) {
				AbortError(c, tc.err)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			e.ServeHTTP(w, req)

			if w.Code != tc.wantHTTP {
				t.Fatalf("http status = %d, want %d", w.Code, tc.wantHTTP)
			}
			var resp Response
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d", resp.Code, tc.wantCode)
			}
			if resp.Data != nil {
				t.Fatalf("data = %v, want nil", resp.Data)
			}
		})
	}
}

// TestWrapPassthrough BizError 原样透传，普通 error 包装为 50001。
func TestWrapPassthrough(t *testing.T) {
	biz := ErrNotFound("赛事不存在")
	if got := Wrap(biz); got != biz {
		t.Fatalf("Wrap(BizError) = %v, want passthrough", got)
	}
	if got := Wrap(nil); got != nil {
		t.Fatalf("Wrap(nil) = %v, want nil", got)
	}
	plain := errors.New("boom")
	got := Wrap(plain)
	if got.Code != CodeInternal || got.Status != http.StatusInternalServerError {
		t.Fatalf("Wrap(plain) = %+v, want 50001/500", got)
	}
	if !errors.Is(got, plain) {
		t.Fatal("Wrap(plain) lost cause chain")
	}
}

// TestOK 成功响应包装格式。
func TestOK(t *testing.T) {
	e := newTestEngine()
	e.GET("/ok", func(c *gin.Context) {
		OK(c, gin.H{"id": 7})
	})

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ok", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Code != CodeOK || resp.Message != "success" {
		t.Fatalf("resp = %+v, want code 0 message success", resp)
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok || data["id"].(float64) != 7 {
		t.Fatalf("data = %v, want {\"id\": 7}", resp.Data)
	}
}

// TestHandleErrorsSkipsWrittenResponse handler 已自行写响应时不重复渲染。
func TestHandleErrorsSkipsWrittenResponse(t *testing.T) {
	e := newTestEngine()
	e.GET("/written", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"legacy": true})
		AbortError(c, ErrInternal(errors.New("late error"), ""))
	})

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/written", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (already written)", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, exists := body["legacy"]; !exists {
		t.Fatalf("body overwritten by error handler: %v", body)
	}
}
