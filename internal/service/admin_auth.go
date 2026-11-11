package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// 登录锁定与审计日志的 action 命名（admin_logs.action）。
const (
	adminActionLogin       = "admin.login"
	adminActionLoginLocked = "admin.login_locked"
	adminActionPassword    = "admin.change_password"
)

// AdminAuthService 负责后台管理员的账号密码登录与密码管理。
type AdminAuthService struct {
	stores *store.Store
	tokens *token.Signer
	guard  *loginGuard
}

// NewAdminAuthService 构造管理员认证服务。
//
// 锁定策略固定为连续 5 次密码错误锁定 15 分钟（API 文档 8.1 注意事项）。
func NewAdminAuthService(stores *store.Store, tokens *token.Signer) *AdminAuthService {
	return &AdminAuthService{
		stores: stores,
		tokens: tokens,
		guard:  newLoginGuard(5, 15*time.Minute, time.Now),
	}
}

// Login 校验账号密码并签发 admin JWT（API 文档 8.1）。
//
// 错误码语义（按原因区分，不复用）：
//   - 参数为空 → 40001
//   - 账号不存在 → 40102
//   - 密码错误 → 40103
//   - 连续密码错误锁定中 → 40104（message 含剩余分钟数）
//   - 账号停用 → 40302
//   - 数据库异常 → 50001
//
// 登录成功刷新 last_login_at 并落 admin_logs。
func (s *AdminAuthService) Login(ctx context.Context, req model.AdminLoginRequest) (*model.AdminLoginResponse, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" || strings.TrimSpace(req.Password) == "" {
		return nil, httpx.ErrBadRequest("账号和密码不能为空")
	}

	if locked, until := s.guard.locked(username); locked {
		// 剩余时间向上取整到分钟，避免提示"0 分钟"。
		mins := int((until.Sub(s.guard.now()) + time.Minute - 1) / time.Minute)
		if mins < 1 {
			mins = 1
		}
		return nil, httpx.ErrAccountLocked(fmt.Sprintf("密码错误次数过多，账号已锁定，请 %d 分钟后再试", mins))
	}

	admin, err := s.stores.Admins.ByUsername(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		// 不存在账号同样累计失败次数（防爆破不以"账号是否存在"区分对待），
		// 触发锁定时与密码错误路径一样落审计日志。
		if s.guard.recordFail(username) {
			s.logLoginLocked(ctx, username)
		}
		return nil, httpx.ErrAccountNotFound("账号不存在")
	}
	if err != nil {
		return nil, httpx.ErrInternal(err, "登录失败，请稍后重试")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		if s.guard.recordFail(username) {
			s.logLoginLocked(ctx, username)
		}
		return nil, httpx.ErrWrongPassword("密码错误")
	}

	if admin.Status == model.AdminStatusDisabled {
		// 密码正确但账号停用：清理失败计数（密码并非错误），明确返回停用。
		s.guard.recordSuccess(username)
		return nil, httpx.ErrAccountDisabled("账号已停用，请联系超级管理员")
	}
	s.guard.recordSuccess(username)

	now := utils.NowISO()
	if err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.stores.Admins.TouchLogin(ctx, tx, admin.ID, now); err != nil {
			return err
		}
		return s.log(ctx, tx, &admin.ID, adminActionLogin, strPtr("admin"), &admin.ID,
			map[string]any{"username": admin.Username, "result": "success"})
	}); err != nil {
		return nil, httpx.ErrInternal(err, "登录失败，请稍后重试")
	}

	signed, err := s.tokens.IssueAdmin(admin.ID, admin.Role)
	if err != nil {
		return nil, httpx.ErrInternal(err, "签发登录凭证失败，请稍后重试")
	}
	return &model.AdminLoginResponse{
		Token:     signed,
		ExpiresIn: int(s.tokens.TTL().Seconds()),
		Admin: model.AdminInfo{
			ID:       admin.ID,
			Username: admin.Username,
			Role:     admin.Role,
			Name:     admin.Name,
		},
	}, nil
}

// ChangePassword 校验旧密码后为当前管理员设置新密码。
//
// 错误码语义：参数缺失/新密码不足 8 位/超过 bcrypt 72 字节上限 → 40001；
// 管理员不存在 → 40101（token 有效但账号已被删除）；旧密码不正确 → 40001；
// 成功后在事务内更新哈希并落 admin_logs。
func (s *AdminAuthService) ChangePassword(ctx context.Context, adminID int64, req model.AdminChangePasswordRequest) error {
	oldPassword, newPassword := req.OldPassword, req.NewPassword
	if strings.TrimSpace(oldPassword) == "" || newPassword == "" {
		return httpx.ErrBadRequest("旧密码和新密码不能为空")
	}
	if len(newPassword) < 8 {
		return httpx.ErrBadRequest("新密码长度至少 8 位")
	}
	if len(newPassword) > 72 {
		return httpx.ErrBadRequest("新密码长度不能超过 72 字节")
	}

	admin, err := s.stores.Admins.Get(ctx, adminID)
	if errors.Is(err, store.ErrNotFound) {
		return httpx.ErrUnauthorized("登录已过期，请重新登录")
	}
	if err != nil {
		return httpx.ErrInternal(err, "修改密码失败，请稍后重试")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(oldPassword)); err != nil {
		return httpx.ErrBadRequest("旧密码不正确")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return httpx.ErrInternal(err, "修改密码失败，请稍后重试")
	}
	if err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.stores.Admins.UpdatePassword(ctx, tx, admin.ID, string(hash)); err != nil {
			return err
		}
		return s.log(ctx, tx, &admin.ID, adminActionPassword, strPtr("admin"), &admin.ID,
			map[string]any{"username": admin.Username})
	}); err != nil {
		return httpx.ErrInternal(err, "修改密码失败，请稍后重试")
	}
	return nil
}

// log 在既有事务内追加一条 admin_logs；detail 序列化为 JSON。
func (s *AdminAuthService) log(ctx context.Context, tx *sql.Tx, adminID *int64, action string, targetType *string, targetID *int64, detail map[string]any) error {
	var raw []byte
	if detail != nil {
		var err error
		raw, err = json.Marshal(detail)
		if err != nil {
			return fmt.Errorf("marshal admin log detail: %w", err)
		}
	}
	return s.stores.Admins.InsertLog(ctx, tx, store.AdminLogParams{
		AdminID:    adminID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     raw,
	})
}

// logLoginLocked 记录登录锁定事件（admin_id 为 nil，表示系统动作）。
func (s *AdminAuthService) logLoginLocked(ctx context.Context, username string) {
	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		return s.log(ctx, tx, nil, adminActionLoginLocked, strPtr("admin"), nil,
			map[string]any{"username": username, "result": "locked"})
	})
	if err != nil {
		// 锁定日志是旁路审计，失败不阻断登录响应，但必须有迹可查。
		logger.FromCtx(ctx).Warnw("write admin login_locked log", "error", err)
	}
}

// loginGuard 是进程内"连续 N 次密码错误锁定 M 分钟"的登录守卫（按用户名）。
//
// 与 middleware.RateLimit（按 IP 的通用限流）互补：RateLimit 挡刷接口，
// loginGuard 挡对单一账号的暴力破解。进程内实现，单机部署足够；
// 多实例部署需换成共享存储。
type loginGuard struct {
	mu          sync.Mutex
	maxFails    int
	lockFor     time.Duration
	now         func() time.Time
	fails       map[string][]time.Time // 最近失败时间戳（新→旧）
	lockedUntil map[string]time.Time   // 锁定截止时间
}

func newLoginGuard(maxFails int, lockFor time.Duration, now func() time.Time) *loginGuard {
	return &loginGuard{
		maxFails:    maxFails,
		lockFor:     lockFor,
		now:         now,
		fails:       make(map[string][]time.Time),
		lockedUntil: make(map[string]time.Time),
	}
}

// locked 返回该用户名是否处于锁定状态及解锁时间。
func (g *loginGuard) locked(username string) (bool, time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.lockedUntil[username]
	if !ok {
		return false, time.Time{}
	}
	if !g.now().Before(until) {
		delete(g.lockedUntil, username)
		delete(g.fails, username)
		return false, time.Time{}
	}
	return true, until
}

// recordFail 记录一次密码错误；返回 true 表示本次失败触发了新锁定。
func (g *loginGuard) recordFail(username string) (lockedNow bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if until, ok := g.lockedUntil[username]; ok && g.now().Before(until) {
		return false // 已锁定，不重复记数/记日志
	}
	cutoff := g.now().Add(-g.lockFor)
	times := g.fails[username]
	kept := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, g.now())
	g.fails[username] = kept
	if len(kept) >= g.maxFails {
		until := g.now().Add(g.lockFor)
		g.lockedUntil[username] = until
		delete(g.fails, username)
		return true
	}
	return false
}

// recordSuccess 登录成功时清理该用户名的失败计数与锁定状态。
func (g *loginGuard) recordSuccess(username string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.fails, username)
	delete(g.lockedUntil, username)
}

func strPtr(s string) *string { return &s }
