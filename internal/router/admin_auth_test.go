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
	"github.com/meetmonth/dance-elite/internal/service"
	"github.com/meetmonth/dance-elite/internal/store"
)

// newAdminAuthTestEngine 打开内存库、跑迁移并装配后台认证路由。
func newAdminAuthTestEngine(t *testing.T) (*gin.Engine, *token.Signer) {
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
	if err := db.Seed(context.Background(), store.SeedOptions{
		AdminUsername: "admin", AdminPassword: "Init@12345",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stores := store.New(db)
	signer := token.NewSigner("test-secret", time.Hour)
	svc := service.NewAdminAuthService(stores, signer)
	return New(Deps{Stores: stores, AdminAuth: svc, Tokens: signer}), signer
}

func postJSON(t *testing.T, engine *gin.Engine, method, path string, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestAdminLoginRoute(t *testing.T) {
	engine, _ := newAdminAuthTestEngine(t)

	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"admin","password":"Init@12345"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Token     string `json:"token"`
			ExpiresIn int    `json:"expires_in"`
			Admin     struct {
				ID       int64   `json:"id"`
				Username string  `json:"username"`
				Role     string  `json:"role"`
				Name     *string `json:"name"`
			} `json:"admin"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != httpx.CodeOK || resp.Data.Token == "" || resp.Data.ExpiresIn != 3600 {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Data.Admin.Username != "admin" || resp.Data.Admin.Role != "super_admin" || resp.Data.Admin.ID == 0 {
		t.Fatalf("admin = %+v", resp.Data.Admin)
	}
	if resp.Data.Admin.Name == nil || *resp.Data.Admin.Name != store.DefaultAdminName {
		t.Fatalf("name = %v, want %q", resp.Data.Admin.Name, store.DefaultAdminName)
	}
}

func TestAdminLoginRouteBadRequestAndUnauthorized(t *testing.T) {
	engine, _ := newAdminAuthTestEngine(t)

	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"admin"}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing password status = %d, want 400", w.Code)
	}
	var resp httpx.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != httpx.CodeBadRequest {
		t.Fatalf("code = %d, want %d", resp.Code, httpx.CodeBadRequest)
	}

	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"admin","password":"wrong"}`, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d, want 401", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != httpx.CodeWrongPassword {
		t.Fatalf("code = %d, want %d", resp.Code, httpx.CodeWrongPassword)
	}

	// 账号不存在与密码错误返回不同错误码。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"ghost","password":"whatever"}`, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown account status = %d, want 401", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != httpx.CodeAccountNotFound {
		t.Fatalf("code = %d, want %d", resp.Code, httpx.CodeAccountNotFound)
	}
}

func TestAdminChangePasswordRoute(t *testing.T) {
	engine, signer := newAdminAuthTestEngine(t)

	// 未带 token → 40101。
	w := postJSON(t, engine, http.MethodPut, "/api/v1/admin/auth/password",
		`{"old_password":"Init@12345","new_password":"NewPass#888"}`, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token status = %d, want 401", w.Code)
	}

	// 用户 token（subject=user）不能冒用后台接口。
	userRaw, err := signer.IssueUser(1, "openid-1")
	if err != nil {
		t.Fatalf("IssueUser: %v", err)
	}
	w = postJSON(t, engine, http.MethodPut, "/api/v1/admin/auth/password",
		`{"old_password":"Init@12345","new_password":"NewPass#888"}`,
		map[string]string{"Authorization": "Bearer " + userRaw})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("user token status = %d, want 401", w.Code)
	}

	// 完整流程：登录 → 改密 → 新密码可登录。
	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"admin","password":"Init@12345"}`, nil)
	var login struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	auth := map[string]string{"Authorization": "Bearer " + login.Data.Token}

	w = postJSON(t, engine, http.MethodPut, "/api/v1/admin/auth/password",
		`{"old_password":"Init@12345","new_password":"NewPass#888"}`, auth)
	if w.Code != http.StatusOK {
		t.Fatalf("change password status = %d, body = %s", w.Code, w.Body)
	}

	w = postJSON(t, engine, http.MethodPost, "/api/v1/admin/auth/login",
		`{"username":"admin","password":"NewPass#888"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login with new password status = %d, body = %s", w.Code, w.Body)
	}

	// 新密码不足 8 位 → 40001。
	w = postJSON(t, engine, http.MethodPut, "/api/v1/admin/auth/password",
		`{"old_password":"NewPass#888","new_password":"short"}`, auth)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("weak new password status = %d, want 400", w.Code)
	}
}
