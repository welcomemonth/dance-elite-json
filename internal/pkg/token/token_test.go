package token

import (
	"errors"
	"testing"
	"time"
)

func TestIssueAndParseUser(t *testing.T) {
	s := NewSigner("test-secret", time.Hour)

	raw, err := s.IssueUser(1024, "openid-1")
	if err != nil {
		t.Fatalf("IssueUser: %v", err)
	}
	claims, err := s.ParseUser(raw)
	if err != nil {
		t.Fatalf("ParseUser: %v", err)
	}
	if claims.UserID != 1024 || claims.OpenID != "openid-1" {
		t.Fatalf("claims = %+v, want user 1024/openid-1", claims)
	}
	if s.TTL() != time.Hour {
		t.Fatalf("TTL = %v, want 1h", s.TTL())
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	raw, err := NewSigner("secret-a", time.Hour).IssueUser(1, "openid")
	if err != nil {
		t.Fatalf("IssueUser: %v", err)
	}
	if _, err := NewSigner("secret-b", time.Hour).ParseUser(raw); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ParseUser error = %v, want ErrInvalidToken", err)
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	raw, err := NewSigner("secret", -time.Minute).IssueUser(1, "openid")
	if err != nil {
		t.Fatalf("IssueUser: %v", err)
	}
	if _, err := NewSigner("secret", time.Hour).ParseUser(raw); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ParseUser error = %v, want ErrInvalidToken", err)
	}
}

func TestParseRejectsMalformedToken(t *testing.T) {
	if _, err := NewSigner("secret", time.Hour).ParseUser("not-a-jwt"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ParseUser error = %v, want ErrInvalidToken", err)
	}
}

func TestIssueAndParseAdmin(t *testing.T) {
	s := NewSigner("test-secret", time.Hour)

	raw, err := s.IssueAdmin(7, "super_admin")
	if err != nil {
		t.Fatalf("IssueAdmin: %v", err)
	}
	claims, err := s.ParseAdmin(raw)
	if err != nil {
		t.Fatalf("ParseAdmin: %v", err)
	}
	if claims.AdminID != 7 || claims.Role != "super_admin" {
		t.Fatalf("claims = %+v, want admin 7/super_admin", claims)
	}
}

// TestParseAdminRejectsUserToken 用户与管理员 token 不可互冒用：
// subject 不同，同一签名器签发的用户 token 不能通过 ParseAdmin。
func TestParseAdminRejectsUserToken(t *testing.T) {
	s := NewSigner("test-secret", time.Hour)
	userRaw, err := s.IssueUser(7, "openid-7")
	if err != nil {
		t.Fatalf("IssueUser: %v", err)
	}
	if _, err := s.ParseAdmin(userRaw); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ParseAdmin(user token) error = %v, want ErrInvalidToken", err)
	}

	adminRaw, err := s.IssueAdmin(7, "admin")
	if err != nil {
		t.Fatalf("IssueAdmin: %v", err)
	}
	if _, err := s.ParseUser(adminRaw); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ParseUser(admin token) error = %v, want ErrInvalidToken", err)
	}
}

func TestParseAdminRejectsExpiredAndMalformed(t *testing.T) {
	expired, err := NewSigner("secret", -time.Minute).IssueAdmin(1, "admin")
	if err != nil {
		t.Fatalf("IssueAdmin: %v", err)
	}
	if _, err := NewSigner("secret", time.Hour).ParseAdmin(expired); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ParseAdmin(expired) error = %v, want ErrInvalidToken", err)
	}
	if _, err := NewSigner("secret", time.Hour).ParseAdmin("not-a-jwt"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ParseAdmin(malformed) error = %v, want ErrInvalidToken", err)
	}
}
