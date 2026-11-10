package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/pkg/wechat"
	"github.com/meetmonth/dance-elite/internal/store"
)

// fakeWechat 按测试场景返回固定的 code2session / 手机号结果。
type fakeWechat struct {
	session    wechat.Session
	sessionErr error
	phone      string
	phoneErr   error
}

func (f *fakeWechat) Code2Session(context.Context, string) (wechat.Session, error) {
	return f.session, f.sessionErr
}

func (f *fakeWechat) PhoneNumber(context.Context, string) (string, error) {
	return f.phone, f.phoneErr
}

func newAuthFixture(t *testing.T, wx WechatClient) (*AuthService, *store.Store, *store.DB) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db.SQL()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	stores := store.New(db)
	svc := NewAuthService(stores, wx, token.NewSigner("test-secret", 2*time.Hour))
	return svc, stores, db
}

func assertBizCode(t *testing.T, err error, want int) {
	t.Helper()
	var biz *httpx.BizError
	if !errors.As(err, &biz) {
		t.Fatalf("error = %v, want *httpx.BizError", err)
	}
	if biz.Code != want {
		t.Fatalf("biz code = %d, want %d (message %q)", biz.Code, want, biz.Message)
	}
}

func TestWxLoginCreatesUser(t *testing.T) {
	wx := &fakeWechat{session: wechat.Session{OpenID: "openid-1"}}
	svc, _, db := newAuthFixture(t, wx)

	resp, err := svc.WxLogin(context.Background(), model.WxLoginRequest{
		Code: "code-1", Nickname: "小雨", AvatarURL: "https://avatar",
	})
	if err != nil {
		t.Fatalf("WxLogin: %v", err)
	}
	if !resp.IsNew {
		t.Error("IsNew = false, want true")
	}
	if resp.Token == "" || resp.ExpiresIn != 7200 {
		t.Fatalf("token/expires_in = %q/%d", resp.Token, resp.ExpiresIn)
	}
	if resp.User.Nickname == nil || *resp.User.Nickname != "小雨" {
		t.Fatalf("nickname = %v", resp.User.Nickname)
	}
	if resp.User.Phone != nil || resp.User.PlayerNo != nil {
		t.Fatalf("phone/player_no = %v/%v, want nil/nil", resp.User.Phone, resp.User.PlayerNo)
	}

	var openid, nickname, lastLogin string
	if err := db.SQL().QueryRow(
		`SELECT openid, nickname, last_login_at FROM users WHERE id = ?`, resp.User.ID,
	).Scan(&openid, &nickname, &lastLogin); err != nil {
		t.Fatalf("query user: %v", err)
	}
	if openid != "openid-1" || nickname != "小雨" || lastLogin == "" {
		t.Fatalf("user row = %q/%q/%q", openid, nickname, lastLogin)
	}
}

func TestWxLoginExistingUserKeepsProfile(t *testing.T) {
	wx := &fakeWechat{session: wechat.Session{OpenID: "openid-1"}}
	svc, _, db := newAuthFixture(t, wx)
	ctx := context.Background()

	first, err := svc.WxLogin(ctx, model.WxLoginRequest{Code: "c1", Nickname: "小雨"})
	if err != nil {
		t.Fatalf("first login: %v", err)
	}

	// 第二次登录：微信未返回昵称/头像，但返回了 unionid
	wx.session = wechat.Session{OpenID: "openid-1", UnionID: "unionid-1"}
	second, err := svc.WxLogin(ctx, model.WxLoginRequest{Code: "c2"})
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if second.IsNew {
		t.Error("IsNew = true, want false")
	}
	if second.User.ID != first.User.ID {
		t.Fatalf("user id changed: %d -> %d", first.User.ID, second.User.ID)
	}
	if second.User.Nickname == nil || *second.User.Nickname != "小雨" {
		t.Fatalf("nickname = %v, want preserved", second.User.Nickname)
	}

	var unionID string
	if err := db.SQL().QueryRow(
		`SELECT unionid FROM users WHERE id = ?`, first.User.ID).Scan(&unionID); err != nil {
		t.Fatalf("query unionid: %v", err)
	}
	if unionID != "unionid-1" {
		t.Fatalf("unionid = %q, want unionid-1", unionID)
	}
}

func TestWxLoginInvalidCode(t *testing.T) {
	wx := &fakeWechat{sessionErr: wechat.ErrInvalidCode}
	svc, _, _ := newAuthFixture(t, wx)

	_, err := svc.WxLogin(context.Background(), model.WxLoginRequest{Code: "bad"})
	assertBizCode(t, err, httpx.CodeBadRequest)
}

func TestWxLoginEmptyCode(t *testing.T) {
	svc, _, _ := newAuthFixture(t, &fakeWechat{})

	_, err := svc.WxLogin(context.Background(), model.WxLoginRequest{Code: "  "})
	assertBizCode(t, err, httpx.CodeBadRequest)
}

func TestWxLoginWechatFailure(t *testing.T) {
	wx := &fakeWechat{sessionErr: errors.New("network down")}
	svc, _, _ := newAuthFixture(t, wx)

	_, err := svc.WxLogin(context.Background(), model.WxLoginRequest{Code: "c1"})
	assertBizCode(t, err, httpx.CodeInternal)
}

func TestWxLoginBlockedUser(t *testing.T) {
	wx := &fakeWechat{session: wechat.Session{OpenID: "openid-1"}}
	svc, _, db := newAuthFixture(t, wx)
	ctx := context.Background()

	resp, err := svc.WxLogin(ctx, model.WxLoginRequest{Code: "c1"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	const untouched = "2020-01-01T00:00:00+08:00"
	if _, err := db.SQL().Exec(
		`UPDATE users SET status = 'blocked', last_login_at = ? WHERE id = ?`,
		untouched, resp.User.ID); err != nil {
		t.Fatalf("block user: %v", err)
	}

	_, err = svc.WxLogin(ctx, model.WxLoginRequest{Code: "c2"})
	assertBizCode(t, err, httpx.CodeForbidden)

	var lastLogin string
	if err := db.SQL().QueryRow(
		`SELECT last_login_at FROM users WHERE id = ?`, resp.User.ID).Scan(&lastLogin); err != nil {
		t.Fatalf("query last_login_at: %v", err)
	}
	if lastLogin != untouched {
		t.Fatalf("last_login_at = %q, want rollback to %q", lastLogin, untouched)
	}
}

func TestWxLoginReturnsLatestPlayerNo(t *testing.T) {
	wx := &fakeWechat{session: wechat.Session{OpenID: "openid-1"}}
	svc, _, db := newAuthFixture(t, wx)
	ctx := context.Background()

	first, err := svc.WxLogin(ctx, model.WxLoginRequest{Code: "c1"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	insertRegistrationFixture(t, db, first.User.ID)

	resp, err := svc.WxLogin(ctx, model.WxLoginRequest{Code: "c2"})
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if resp.User.PlayerNo == nil || *resp.User.PlayerNo != "Ab12Cd34" {
		t.Fatalf("player_no = %v, want Ab12Cd34", resp.User.PlayerNo)
	}
}

func TestBindPhone(t *testing.T) {
	wx := &fakeWechat{session: wechat.Session{OpenID: "openid-1"}, phone: "13800138000"}
	svc, _, db := newAuthFixture(t, wx)
	ctx := context.Background()

	login, err := svc.WxLogin(ctx, model.WxLoginRequest{Code: "c1"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	resp, err := svc.BindPhone(ctx, login.User.ID, "phone-code")
	if err != nil {
		t.Fatalf("BindPhone: %v", err)
	}
	if resp.Phone != "138****8000" {
		t.Fatalf("masked phone = %q", resp.Phone)
	}

	var stored string
	if err := db.SQL().QueryRow(
		`SELECT phone FROM users WHERE id = ?`, login.User.ID).Scan(&stored); err != nil {
		t.Fatalf("query phone: %v", err)
	}
	if stored != "13800138000" {
		t.Fatalf("stored phone = %q, want plaintext", stored)
	}
}

func TestBindPhoneInvalidCode(t *testing.T) {
	wx := &fakeWechat{phoneErr: wechat.ErrInvalidCode}
	svc, _, _ := newAuthFixture(t, wx)

	_, err := svc.BindPhone(context.Background(), 1, "bad")
	assertBizCode(t, err, httpx.CodeBadRequest)
}

func TestBindPhoneUnknownUser(t *testing.T) {
	wx := &fakeWechat{phone: "13800138000"}
	svc, _, _ := newAuthFixture(t, wx)

	_, err := svc.BindPhone(context.Background(), 999, "phone-code")
	assertBizCode(t, err, httpx.CodeUnauthorized)
}

// insertRegistrationFixture 写入一条 active 报名，用于验证默认 player_no。
func insertRegistrationFixture(t *testing.T, db *store.DB, userID int64) {
	t.Helper()
	stmts := []string{
		`INSERT INTO seasons (id, name) VALUES (1, '2027 赛季')`,
		`INSERT INTO events (id, season_id, name, level) VALUES (1, 1, '测试赛', 'grade_a')`,
		`INSERT INTO event_groups (id, event_id, level, dance_type, birth_year_from, birth_year_to, quota)
		 VALUES (1, 1, 'U11', 'classical', 2016, 2017, 10)`,
		`INSERT INTO players (id, player_no, name) VALUES (1, 'Ab12Cd34', '张小雨')`,
		`INSERT INTO registrations (event_id, group_id, player_id, user_id, dance_type, status)
		 VALUES (1, 1, 1, ?, 'classical', 'active')`,
	}
	for i, stmt := range stmts {
		var err error
		if i == len(stmts)-1 {
			_, err = db.SQL().Exec(stmt, userID)
		} else {
			_, err = db.SQL().Exec(stmt)
		}
		if err != nil {
			t.Fatalf("fixture stmt %d: %v", i, err)
		}
	}
}
