package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/pkg/wechat"
	"github.com/meetmonth/dance-elite/internal/service"
	"github.com/meetmonth/dance-elite/internal/store"
)

type stubWechat struct {
	session wechat.Session
}

func (s *stubWechat) Code2Session(context.Context, string) (wechat.Session, error) {
	return s.session, nil
}

func (s *stubWechat) PhoneNumber(context.Context, string) (string, error) {
	return "13800138000", nil
}

func newAuthTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db.SQL()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	stores := store.New(db)
	signer := token.NewSigner("test-secret", time.Hour)
	svc := service.NewAuthService(stores, &stubWechat{session: wechat.Session{OpenID: "openid-1"}}, signer)
	return New(Deps{Stores: stores, Auth: svc, Tokens: signer})
}

func TestWxLoginRoute(t *testing.T) {
	engine := newAuthTestEngine(t)
	body := bytes.NewBufferString(`{"code":"code-1","nickname":"小雨"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/wx-login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Token     string `json:"token"`
			ExpiresIn int    `json:"expires_in"`
			IsNew     bool   `json:"is_new"`
			User      struct {
				ID int64 `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != httpx.CodeOK {
		t.Fatalf("code = %d", resp.Code)
	}
	if resp.Data.Token == "" || resp.Data.ExpiresIn != 3600 || !resp.Data.IsNew || resp.Data.User.ID == 0 {
		t.Fatalf("data = %+v", resp.Data)
	}
}

func TestWxLoginRouteMissingCode(t *testing.T) {
	engine := newAuthTestEngine(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/wx-login",
		bytes.NewBufferString(`{"nickname":"小雨"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var resp httpx.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != httpx.CodeBadRequest {
		t.Fatalf("code = %d, want %d", resp.Code, httpx.CodeBadRequest)
	}
}

func TestBindPhoneRouteRequiresLogin(t *testing.T) {
	engine := newAuthTestEngine(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/phone",
		bytes.NewBufferString(`{"phone_code":"abc"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
