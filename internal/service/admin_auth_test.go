package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/store"
)

// newAdminAuthFixture 打开内存库、跑迁移并种子超管 admin/Init@12345。
func newAdminAuthFixture(t *testing.T) (*AdminAuthService, *store.Store, *store.DB) {
	t.Helper()
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
	return NewAdminAuthService(stores, token.NewSigner("test-secret", 2*time.Hour)), stores, db
}

func loginReq(password string) model.AdminLoginRequest {
	return model.AdminLoginRequest{Username: "admin", Password: password}
}

func TestAdminLoginSuccess(t *testing.T) {
	svc, _, db := newAdminAuthFixture(t)

	resp, err := svc.Login(context.Background(), loginReq("Init@12345"))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.Token == "" || resp.ExpiresIn != 7200 {
		t.Fatalf("token/expires_in = %q/%d", resp.Token, resp.ExpiresIn)
	}
	if resp.Admin.ID == 0 || resp.Admin.Username != "admin" || resp.Admin.Role != "super_admin" {
		t.Fatalf("admin = %+v", resp.Admin)
	}
	if resp.Admin.Name == nil || *resp.Admin.Name != store.DefaultAdminName {
		t.Fatalf("name = %v, want %q", resp.Admin.Name, store.DefaultAdminName)
	}

	// 签发的 token 必须能通过 ParseAdmin 还原身份。
	claims, err := token.NewSigner("test-secret", time.Hour).ParseAdmin(resp.Token)
	if err != nil {
		t.Fatalf("ParseAdmin: %v", err)
	}
	if claims.AdminID != resp.Admin.ID || claims.Role != "super_admin" {
		t.Fatalf("claims = %+v", claims)
	}

	// last_login_at 已刷新，且落了登录日志。
	var lastLogin, action string
	if err := db.SQL().QueryRow(
		`SELECT last_login_at FROM admins WHERE id = ?`, resp.Admin.ID).Scan(&lastLogin); err != nil {
		t.Fatalf("query last_login_at: %v", err)
	}
	if lastLogin == "" {
		t.Fatal("last_login_at not refreshed")
	}
	if err := db.SQL().QueryRow(
		`SELECT action FROM admin_logs WHERE admin_id = ?`, resp.Admin.ID).Scan(&action); err != nil {
		t.Fatalf("query admin_logs: %v", err)
	}
	if action != "admin.login" {
		t.Fatalf("log action = %q, want admin.login", action)
	}
}

func TestAdminLoginWrongPasswordAndUnknownUser(t *testing.T) {
	svc, _, _ := newAdminAuthFixture(t)

	_, err := svc.Login(context.Background(), loginReq("wrong-password"))
	assertBizCode(t, err, httpx.CodeWrongPassword)

	_, err = svc.Login(context.Background(), model.AdminLoginRequest{
		Username: "ghost", Password: "whatever",
	})
	assertBizCode(t, err, httpx.CodeAccountNotFound)
}

// TestAdminLoginLockOnUnknownUserLogs 不存在账号同样累计失败并触发锁定，
// 锁定事件必须落 admin.login_locked 审计日志（与密码错误路径一致）。
func TestAdminLoginLockOnUnknownUserLogs(t *testing.T) {
	svc, _, db := newAdminAuthFixture(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := svc.Login(ctx, model.AdminLoginRequest{Username: "ghost", Password: "whatever"})
		assertBizCode(t, err, httpx.CodeAccountNotFound)
	}

	// 第 6 次返回锁定；锁定来自"账号不存在"路径，日志也应写入。
	_, err := svc.Login(ctx, model.AdminLoginRequest{Username: "ghost", Password: "whatever"})
	assertBizCode(t, err, httpx.CodeAccountLocked)

	var count int
	if err := db.SQL().QueryRow(
		`SELECT COUNT(*) FROM admin_logs WHERE action = 'admin.login_locked'`).Scan(&count); err != nil {
		t.Fatalf("count lock logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("lock logs = %d, want 1", count)
	}
}

func TestAdminLoginEmptyParams(t *testing.T) {
	svc, _, _ := newAdminAuthFixture(t)

	_, err := svc.Login(context.Background(), model.AdminLoginRequest{Username: "  ", Password: "x"})
	assertBizCode(t, err, httpx.CodeBadRequest)

	_, err = svc.Login(context.Background(), model.AdminLoginRequest{Username: "admin", Password: " "})
	assertBizCode(t, err, httpx.CodeBadRequest)
}

func TestAdminLoginDisabled(t *testing.T) {
	svc, _, db := newAdminAuthFixture(t)
	if _, err := db.SQL().Exec(
		`UPDATE admins SET status = 'disabled' WHERE username = 'admin'`); err != nil {
		t.Fatalf("disable admin: %v", err)
	}

	_, err := svc.Login(context.Background(), loginReq("Init@12345"))
	assertBizCode(t, err, httpx.CodeAccountDisabled)
}

// TestAdminLoginLockout 连续 5 次密码错误锁定 15 分钟：锁定期内即使用正确
// 密码也拒绝，到期后自动解锁。
func TestAdminLoginLockout(t *testing.T) {
	svc, _, db := newAdminAuthFixture(t)
	ctx := context.Background()

	// 注入可控时钟，避免测试真实等待 15 分钟。
	start := time.Now()
	current := start
	svc.guard = newLoginGuard(5, 15*time.Minute, func() time.Time { return current })

	for i := 0; i < 5; i++ {
		_, err := svc.Login(ctx, loginReq("wrong-password"))
		assertBizCode(t, err, httpx.CodeWrongPassword)
	}

	// 第 6 次即使用正确密码也被锁定，并提示剩余分钟数。
	_, err := svc.Login(ctx, loginReq("Init@12345"))
	assertBizCode(t, err, httpx.CodeAccountLocked)
	var biz *httpx.BizError
	if !errors.As(err, &biz) || !strings.Contains(biz.Message, "15 分钟") {
		t.Fatalf("lock error = %v, want message containing remaining minutes", err)
	}

	// 锁定事件落了系统日志（admin_id 为空）。
	var count int
	if err := db.SQL().QueryRow(
		`SELECT COUNT(*) FROM admin_logs WHERE action = 'admin.login_locked'`).Scan(&count); err != nil {
		t.Fatalf("count lock logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("lock logs = %d, want 1", count)
	}

	// 15 分钟后自动解锁，可用正确密码登录。
	current = start.Add(16 * time.Minute)
	resp, err := svc.Login(ctx, loginReq("Init@12345"))
	if err != nil {
		t.Fatalf("Login after lock expired: %v", err)
	}
	if resp.Admin.Username != "admin" {
		t.Fatalf("admin = %+v", resp.Admin)
	}
}

// TestAdminLoginFailCounterResetsOnSuccess 登录成功清零失败计数，
// 避免"错 4 次 + 对 1 次 + 错 1 次"被误判锁定。
func TestAdminLoginFailCounterResetsOnSuccess(t *testing.T) {
	svc, _, _ := newAdminAuthFixture(t)
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		_, err := svc.Login(ctx, loginReq("wrong-password"))
		assertBizCode(t, err, httpx.CodeWrongPassword)
	}
	if _, err := svc.Login(ctx, loginReq("Init@12345")); err != nil {
		t.Fatalf("Login: %v", err)
	}
	// 成功后再错 4 次（<5）不应锁定。
	for i := 0; i < 4; i++ {
		_, err := svc.Login(ctx, loginReq("wrong-password"))
		assertBizCode(t, err, httpx.CodeWrongPassword)
	}
	if _, err := svc.Login(ctx, loginReq("Init@12345")); err != nil {
		t.Fatalf("Login should not be locked: %v", err)
	}
}

func TestAdminChangePassword(t *testing.T) {
	svc, _, db := newAdminAuthFixture(t)
	ctx := context.Background()

	login, err := svc.Login(ctx, loginReq("Init@12345"))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	err = svc.ChangePassword(ctx, login.Admin.ID, model.AdminChangePasswordRequest{
		OldPassword: "Init@12345", NewPassword: "NewPass#888",
	})
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	// 旧密码失效、新密码可登录。
	if _, err := svc.Login(ctx, loginReq("Init@12345")); err == nil {
		t.Fatal("login with old password should fail")
	}
	if _, err := svc.Login(ctx, loginReq("NewPass#888")); err != nil {
		t.Fatalf("login with new password: %v", err)
	}

	// 密码已落库为 bcrypt 哈希，且写入了改密日志。
	admin, err := svc.stores.Admins.ByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("ByUsername: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("NewPass#888")); err != nil {
		t.Fatal("stored hash does not match new password")
	}
	var count int
	if err := db.SQL().QueryRow(
		`SELECT COUNT(*) FROM admin_logs WHERE action = 'admin.change_password'`).Scan(&count); err != nil {
		t.Fatalf("count change logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("change_password logs = %d, want 1", count)
	}
}

func TestAdminChangePasswordValidation(t *testing.T) {
	svc, _, _ := newAdminAuthFixture(t)
	ctx := context.Background()
	login, err := svc.Login(ctx, loginReq("Init@12345"))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	cases := []model.AdminChangePasswordRequest{
		{OldPassword: "Init@12345", NewPassword: "short"},      // 不足 8 位
		{OldPassword: "wrong-old", NewPassword: "NewPass#888"}, // 旧密码不正确
		{OldPassword: "Init@12345", NewPassword: ""},           // 新密码为空
	}
	for i, req := range cases {
		if err := svc.ChangePassword(ctx, login.Admin.ID, req); err == nil {
			t.Fatalf("case %d: ChangePassword(%+v) should fail", i, req)
		} else {
			assertBizCode(t, err, httpx.CodeBadRequest)
		}
	}

	// 管理员不存在（token 有效但账号已删）→ 40101。
	err = svc.ChangePassword(ctx, 999, model.AdminChangePasswordRequest{
		OldPassword: "Init@12345", NewPassword: "NewPass#888",
	})
	assertBizCode(t, err, httpx.CodeUnauthorized)

	// 校验失败后原密码仍然可用。
	if _, err := svc.Login(ctx, loginReq("Init@12345")); err != nil {
		t.Fatalf("login after failed changes: %v", err)
	}
}
