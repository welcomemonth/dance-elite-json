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
