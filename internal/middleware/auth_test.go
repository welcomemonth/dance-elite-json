package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
)

func newTestEngine(handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	// HandleErrors 必须先于业务中间件注册：它在 c.Next() 返回后统一渲染 c.Errors。
	chain := append([]gin.HandlerFunc{httpx.HandleErrors()}, handlers...)
	e.Use(chain...)
	e.GET("/me", func(c *gin.Context) {
		userID, _ := GetUserID(c)
		openID, _ := GetOpenID(c)
		httpx.OK(c, gin.H{"user_id": userID, "openid": openID})
	})
	return e
}

func doRequest(e *gin.Engine, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func TestUserAuthAcceptsValidToken(t *testing.T) {
	signer := token.NewSigner("test-secret", time.Hour)
	raw, err := signer.IssueUser(42, "openid-42")
	if err != nil {
		t.Fatalf("IssueUser: %v", err)
	}

	w := doRequest(newTestEngine(UserAuth(signer)), "Bearer "+raw)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body)
	}
	var body struct {
		Data struct {
			UserID int64  `json:"user_id"`
			OpenID string `json:"openid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Data.UserID != 42 || body.Data.OpenID != "openid-42" {
		t.Fatalf("data = %+v", body.Data)
	}
}

func TestUserAuthRejectsMissingOrInvalidToken(t *testing.T) {
	signer := token.NewSigner("test-secret", time.Hour)
	other, err := token.NewSigner("other-secret", time.Hour).IssueUser(1, "openid")
	if err != nil {
		t.Fatalf("IssueUser: %v", err)
	}

	for name, header := range map[string]string{
		"missing":    "",
		"no scheme":  "abc.def.ghi",
		"bad secret": "Bearer " + other,
		"malformed":  "Bearer not-a-jwt",
	} {
		t.Run(name, func(t *testing.T) {
			w := doRequest(newTestEngine(UserAuth(signer)), header)
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

func TestRateLimit(t *testing.T) {
	e := newTestEngine(RateLimit(2, time.Minute))

	for i := 0; i < 2; i++ {
		if w := doRequest(e, ""); w.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i+1, w.Code)
		}
	}
	w := doRequest(e, "")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("third request status = %d, want 429", w.Code)
	}
	var body httpx.Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Code != httpx.CodeTooManyRequests {
		t.Fatalf("code = %d, want %d", body.Code, httpx.CodeTooManyRequests)
	}
}
