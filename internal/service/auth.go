// Package service 是业务逻辑层：接收 store 与外部客户端，返回用户可见错误
// （httpx.BizError）或内部错误，由 handler 统一渲染。
package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/pkg/wechat"
	"github.com/meetmonth/dance-elite/internal/store"
	"github.com/meetmonth/dance-elite/internal/utils"
)

// WechatClient 是认证服务依赖的微信接口，*wechat.Client 实现；
// 测试注入 fake，覆盖微信侧各种返回。
type WechatClient interface {
	Code2Session(ctx context.Context, code string) (wechat.Session, error)
	PhoneNumber(ctx context.Context, code string) (string, error)
}

// AuthService 负责小程序静默登录与手机号绑定。
type AuthService struct {
	stores *store.Store
	wechat WechatClient
	tokens *token.Signer
}

// NewAuthService 构造认证服务。
func NewAuthService(stores *store.Store, wx WechatClient, tokens *token.Signer) *AuthService {
	return &AuthService{stores: stores, wechat: wx, tokens: tokens}
}

// WxLogin 用 wx.login 的 code 换取 openid，按 openid 建号/更新并签发 JWT。
//
// 错误码语义：code 无效 → 40001；账号被封禁 → 40301；微信或数据库异常 → 50001。
func (s *AuthService) WxLogin(ctx context.Context, req model.WxLoginRequest) (*model.WxLoginResponse, error) {
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return nil, httpx.ErrBadRequest("code 不能为空")
	}

	session, err := s.wechat.Code2Session(ctx, code)
	if err != nil {
		if errors.Is(err, wechat.ErrInvalidCode) {
			return nil, httpx.ErrBadRequest("code 已失效，请重新登录")
		}
		return nil, httpx.ErrInternal(err, "微信登录失败，请稍后重试")
	}
	if session.OpenID == "" {
		return nil, httpx.ErrInternal(errors.New("wechat returned empty openid"), "微信登录失败，请稍后重试")
	}

	var user *model.User
	var created bool
	err = s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		u, isNew, err := s.stores.Users.UpsertLogin(ctx, tx, store.LoginParams{
			OpenID:    session.OpenID,
			UnionID:   session.UnionID,
			Nickname:  strings.TrimSpace(req.Nickname),
			AvatarURL: strings.TrimSpace(req.AvatarURL),
			Now:       utils.NowISO(),
		})
		if err != nil {
			return err
		}
		if u.Status == model.UserStatusBlocked {
			// 返回错误令事务回滚，封禁用户不刷新 last_login_at。
			return httpx.ErrForbidden("账号已被禁用，请联系客服")
		}
		user, created = u, isNew
		return nil
	})
	if err != nil {
		return nil, err
	}

	signed, err := s.tokens.IssueUser(user.ID, user.OpenID)
	if err != nil {
		return nil, httpx.ErrInternal(err, "签发登录凭证失败，请稍后重试")
	}
	playerNo, err := s.stores.Users.DefaultPlayerNo(ctx, user.ID)
	if err != nil {
		return nil, httpx.ErrInternal(err, "读取用户信息失败，请稍后重试")
	}

	return &model.WxLoginResponse{
		Token:     signed,
		ExpiresIn: int(s.tokens.TTL().Seconds()),
		IsNew:     created,
		User: model.UserInfo{
			ID:        user.ID,
			Nickname:  user.Nickname,
			AvatarURL: user.AvatarURL,
			Phone:     maskedPhone(user.Phone),
			PlayerNo:  playerNo,
		},
	}, nil
}

// BindPhone 用 getPhoneNumber 的 code 换取手机号并绑定到当前用户。
// 手机号明文存储，响应只返回脱敏值。
func (s *AuthService) BindPhone(ctx context.Context, userID int64, phoneCode string) (*model.BindPhoneResponse, error) {
	phoneCode = strings.TrimSpace(phoneCode)
	if phoneCode == "" {
		return nil, httpx.ErrBadRequest("phone_code 不能为空")
	}

	phone, err := s.wechat.PhoneNumber(ctx, phoneCode)
	if err != nil {
		if errors.Is(err, wechat.ErrInvalidCode) {
			return nil, httpx.ErrBadRequest("手机号授权已失效，请重新授权")
		}
		return nil, httpx.ErrInternal(err, "获取手机号失败，请稍后重试")
	}
	if phone == "" {
		return nil, httpx.ErrInternal(errors.New("wechat returned empty phone"), "获取手机号失败，请稍后重试")
	}

	if err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		return s.stores.Users.UpdatePhone(ctx, tx, userID, phone)
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.ErrUnauthorized("登录已过期，请重新登录")
		}
		return nil, httpx.ErrInternal(err, "绑定手机号失败，请稍后重试")
	}

	return &model.BindPhoneResponse{Phone: utils.MaskPhone(phone)}, nil
}

func maskedPhone(phone *string) *string {
	if phone == nil || *phone == "" {
		return nil
	}
	masked := utils.MaskPhone(*phone)
	return &masked
}
