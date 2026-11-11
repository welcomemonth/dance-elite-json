package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
)

// newAdminTestEngine 构造 AdminAuth + RequireRole 的测试链，命中后回显身份。
func newAdminTestEngine(signer *token.Signer, roles ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(httpx.HandleErrors(), AdminAuth(signer))
	if len(roles) > 0 {
		e.Use(RequireRole(roles...))
	}
	e.GET("/whoami", func(c *gin.Context) {
		adminID, _ := GetAdminID(c)
		role, _ := GetAdminRole(c)
		httpx.OK(c, gin.H{"admin_id": adminID, "role": role})
	})
	return e
}

func doAdminRequest(e *gin.Engine, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func TestAdminAuthAcceptsValidToken(t *testing.T) {
	signer := token.NewSigner("test-secret", time.Hour)
	raw, err := signer.IssueAdmin(9, "cs")
	if err != nil {
		t.Fatalf("IssueAdmin: %v", err)
	}

	w := doAdminRequest(newAdminTestEngine(signer), "Bearer "+raw)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
	var body struct {
		Data struct {
			AdminID int64  `json:"admin_id"`
			Role    string `json:"role"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Data.AdminID != 9 || body.Data.Role != "cs" {
		t.Fatalf("data = %+v, want admin 9/cs", body.Data)
	}
}

func TestAdminAuthRejectsMissingInvalidAndUserToken(t *testing.T) {
	signer := token.NewSigner("test-secret", time.Hour)
	userRaw, err := signer.IssueUser(9, "openid-9")
	if err != nil {
		t.Fatalf("IssueUser: %v", err)
	}

	for name, header := range map[string]string{
		"missing":    "",
		"malformed":  "Bearer not-a-jwt",
		"user token": "Bearer " + userRaw, // subject 不匹配，不可冒用
	} {
		t.Run(name, func(t *testing.T) {
			w := doAdminRequest(newAdminTestEngine(signer), header)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", w.Code)
			}
			var body httpx.Response
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Code != httpx.CodeUnauthorized {
				t.Fatalf("code = %d, want %d", body.Code, httpx.CodeUnauthorized)
			}
		})
	}
}

func TestRequireRoleAllowsAndDenies(t *testing.T) {
	signer := token.NewSigner("test-secret", time.Hour)
	issue := func(role string) string {
		raw, err := signer.IssueAdmin(1, role)
		if err != nil {
			t.Fatalf("IssueAdmin(%s): %v", role, err)
		}
		return "Bearer " + raw
	}

	cases := []struct {
		name     string
		guard    []string
		role     string
		wantHTTP int
	}{
		{"super_admin in super+admin", []string{model.RoleSuperAdmin, model.RoleAdmin}, model.RoleSuperAdmin, http.StatusOK},
		{"admin in super+admin", []string{model.RoleSuperAdmin, model.RoleAdmin}, model.RoleAdmin, http.StatusOK},
		{"cs denied on super+admin", []string{model.RoleSuperAdmin, model.RoleAdmin}, model.RoleCS, http.StatusForbidden},
		{"cs allowed on cs", []string{model.RoleCS}, model.RoleCS, http.StatusOK},
		{"admin denied on cs-only", []string{model.RoleCS}, model.RoleAdmin, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doAdminRequest(newAdminTestEngine(signer, tc.guard...), issue(tc.role))
			if w.Code != tc.wantHTTP {
				t.Fatalf("status = %d, want %d, body = %s", w.Code, tc.wantHTTP, w.Body)
			}
			var body httpx.Response
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			wantCode := httpx.CodeOK
			if tc.wantHTTP == http.StatusForbidden {
				wantCode = httpx.CodeForbidden
			}
			if body.Code != wantCode {
				t.Fatalf("code = %d, want %d", body.Code, wantCode)
			}
		})
	}
}
