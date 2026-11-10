package wechat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestCode2SessionSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sns/jscode2session" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("appid") != "wx-app" || q.Get("secret") != "wx-secret" ||
			q.Get("js_code") != "code-1" || q.Get("grant_type") != "authorization_code" {
			t.Errorf("unexpected query: %v", q)
		}
		_, _ = w.Write([]byte(`{"openid":"openid-1","unionid":"unionid-1","session_key":"do-not-leak"}`))
	}))
	defer server.Close()

	c := NewClient("wx-app", "wx-secret", WithBaseURL(server.URL))
	got, err := c.Code2Session(context.Background(), "code-1")
	if err != nil {
		t.Fatalf("Code2Session: %v", err)
	}
	if got.OpenID != "openid-1" || got.UnionID != "unionid-1" {
		t.Fatalf("session = %+v", got)
	}
}

func TestCode2SessionInvalidCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":40029,"errmsg":"invalid code"}`))
	}))
	defer server.Close()

	c := NewClient("wx-app", "wx-secret", WithBaseURL(server.URL))
	_, err := c.Code2Session(context.Background(), "bad")
	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("error = %v, want ErrInvalidCode", err)
	}
}

func TestCode2SessionHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient("wx-app", "wx-secret", WithBaseURL(server.URL))
	_, err := c.Code2Session(context.Background(), "code")
	if err == nil || errors.Is(err, ErrInvalidCode) {
		t.Fatalf("error = %v, want non-invalid-code error", err)
	}
}

func TestPhoneNumberCachesAccessToken(t *testing.T) {
	var tokenCalls, phoneCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/stable_token":
			tokenCalls.Add(1)
			writeJSON(t, w, map[string]any{"access_token": "token-1", "expires_in": 7200})
		case "/wxa/business/getuserphonenumber":
			phoneCalls.Add(1)
			if got := r.URL.Query().Get("access_token"); got != "token-1" {
				t.Errorf("access_token = %q, want token-1", got)
			}
			writeJSON(t, w, map[string]any{
				"errcode": 0,
				"phone_info": map[string]string{
					"phoneNumber":     "+8613800138000",
					"purePhoneNumber": "13800138000",
				},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	c := NewClient("wx-app", "wx-secret", WithBaseURL(server.URL))
	for i := 0; i < 2; i++ {
		phone, err := c.PhoneNumber(context.Background(), "phone-code")
		if err != nil {
			t.Fatalf("PhoneNumber: %v", err)
		}
		if phone != "13800138000" {
			t.Fatalf("phone = %q", phone)
		}
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token endpoint called %d times, want 1 (cached)", tokenCalls.Load())
	}
	if phoneCalls.Load() != 2 {
		t.Fatalf("phone endpoint called %d times, want 2", phoneCalls.Load())
	}
}

func TestPhoneNumberRefreshesInvalidAccessToken(t *testing.T) {
	var tokenCalls, phoneCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/stable_token":
			n := tokenCalls.Add(1)
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode token request: %v", err)
			}
			if n > 1 && req["force_refresh"] != true {
				t.Errorf("retry force_refresh = %v, want true", req["force_refresh"])
			}
			writeJSON(t, w, map[string]any{"access_token": fmt.Sprintf("token-%d", n), "expires_in": 7200})
		case "/wxa/business/getuserphonenumber":
			if phoneCalls.Add(1) == 1 {
				writeJSON(t, w, map[string]any{"errcode": 40001, "errmsg": "invalid credential"})
				return
			}
			writeJSON(t, w, map[string]any{
				"errcode":    0,
				"phone_info": map[string]string{"purePhoneNumber": "13800138000"},
			})
		}
	}))
	defer server.Close()

	c := NewClient("wx-app", "wx-secret", WithBaseURL(server.URL))
	phone, err := c.PhoneNumber(context.Background(), "phone-code")
	if err != nil {
		t.Fatalf("PhoneNumber: %v", err)
	}
	if phone != "13800138000" {
		t.Fatalf("phone = %q", phone)
	}
	if tokenCalls.Load() != 2 || phoneCalls.Load() != 2 {
		t.Fatalf("token calls = %d, phone calls = %d, want 2/2", tokenCalls.Load(), phoneCalls.Load())
	}
}

func TestPhoneNumberInvalidCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/stable_token":
			writeJSON(t, w, map[string]any{"access_token": "token-1", "expires_in": 7200})
		case "/wxa/business/getuserphonenumber":
			writeJSON(t, w, map[string]any{"errcode": 40029, "errmsg": "invalid code"})
		}
	}))
	defer server.Close()

	c := NewClient("wx-app", "wx-secret", WithBaseURL(server.URL))
	_, err := c.PhoneNumber(context.Background(), "bad")
	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("error = %v, want ErrInvalidCode", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
