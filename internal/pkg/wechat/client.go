// Package wechat 封装小程序服务端调用的微信开放接口：
//   - code2session：wx.login 的 code 换 openid/unionid（无需 access_token）；
//   - stable_token + getuserphonenumber：getPhoneNumber 的 code 换手机号（需 access_token）。
//
// access_token 在进程内缓存并提前 5 分钟刷新；遇到 token 失效错误时强制刷新一次。
package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultBaseURL = "https://api.weixin.qq.com"

// maxResponseBytes 限制微信响应体大小，防止异常响应占满内存。
const maxResponseBytes = 1 << 20

var (
	// ErrInvalidCode 表示 code 无效、已使用或已过期（对外映射为参数错误）。
	ErrInvalidCode = errors.New("wechat: invalid code")
	// errAccessTokenInvalid 是内部错误：access_token 失效，需要强制刷新后重试。
	errAccessTokenInvalid = errors.New("wechat: access token invalid")
)

// Session 是 code2session 的登录态信息。session_key 不落库、不返回、不记录日志。
type Session struct {
	OpenID  string
	UnionID string
}

// Option 定制 Client（测试用）。
type Option func(*Client)

// WithHTTPClient 替换默认 HTTP 客户端。
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// WithBaseURL 替换微信 API 根地址（测试指向 httptest 服务）。
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

// Client 是微信接口客户端，并发安全。
type Client struct {
	appID   string
	secret  string
	baseURL string
	http    *http.Client

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

// NewClient 构造微信客户端；HTTP 超时 10 秒。
func NewClient(appID, secret string, opts ...Option) *Client {
	c := &Client{
		appID:   appID,
		secret:  secret,
		baseURL: defaultBaseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Code2Session 调用 jscode2session 换取 openid/unionid。
//
// code 无效/已使用时返回 ErrInvalidCode；网络或微信侧异常返回其余错误。
func (c *Client) Code2Session(ctx context.Context, code string) (Session, error) {
	if strings.TrimSpace(code) == "" {
		return Session{}, fmt.Errorf("%w: empty code", ErrInvalidCode)
	}

	q := url.Values{}
	q.Set("appid", c.appID)
	q.Set("secret", c.secret)
	q.Set("js_code", code)
	q.Set("grant_type", "authorization_code")

	var resp struct {
		OpenID     string `json:"openid"`
		UnionID    string `json:"unionid"`
		SessionKey string `json:"session_key"`
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
	}
	if err := c.getJSON(ctx, "/sns/jscode2session?"+q.Encode(), &resp); err != nil {
		return Session{}, fmt.Errorf("wechat code2session: %w", err)
	}

	switch resp.ErrCode {
	case 0:
	case 40029, 40163: // invalid code / code been used
		return Session{}, fmt.Errorf("%w: errcode=%d %s", ErrInvalidCode, resp.ErrCode, resp.ErrMsg)
	default:
		return Session{}, fmt.Errorf("wechat code2session: errcode=%d %s", resp.ErrCode, resp.ErrMsg)
	}
	if resp.OpenID == "" {
		return Session{}, errors.New("wechat code2session: empty openid")
	}
	return Session{OpenID: resp.OpenID, UnionID: resp.UnionID}, nil
}

// PhoneNumber 调用 getuserphonenumber 用手机号授权 code 换取手机号。
//
// code 无效/已使用时返回 ErrInvalidCode；access_token 失效时自动强制刷新并重试一次。
func (c *Client) PhoneNumber(ctx context.Context, code string) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", fmt.Errorf("%w: empty phone code", ErrInvalidCode)
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		accessToken, err := c.accessToken(ctx, attempt > 0)
		if err != nil {
			return "", err
		}

		phone, err := c.requestPhone(ctx, accessToken, code)
		if err == nil {
			return phone, nil
		}
		lastErr = err
		if !errors.Is(err, errAccessTokenInvalid) {
			return "", err
		}
	}
	return "", lastErr
}

func (c *Client) requestPhone(ctx context.Context, accessToken, code string) (string, error) {
	body, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		return "", fmt.Errorf("encode phone request: %w", err)
	}

	var resp struct {
		ErrCode   int    `json:"errcode"`
		ErrMsg    string `json:"errmsg"`
		PhoneInfo struct {
			PhoneNumber     string `json:"phoneNumber"`
			PurePhoneNumber string `json:"purePhoneNumber"`
		} `json:"phone_info"`
	}
	endpoint := "/wxa/business/getuserphonenumber?access_token=" + url.QueryEscape(accessToken)
	if err := c.postJSON(ctx, endpoint, body, &resp); err != nil {
		return "", fmt.Errorf("wechat getuserphonenumber: %w", err)
	}

	switch resp.ErrCode {
	case 0:
	case 40001, 40014, 42001: // invalid / expired access token
		return "", fmt.Errorf("%w: errcode=%d %s", errAccessTokenInvalid, resp.ErrCode, resp.ErrMsg)
	case 40029, 40163:
		return "", fmt.Errorf("%w: errcode=%d %s", ErrInvalidCode, resp.ErrCode, resp.ErrMsg)
	default:
		return "", fmt.Errorf("wechat getuserphonenumber: errcode=%d %s", resp.ErrCode, resp.ErrMsg)
	}

	if resp.PhoneInfo.PurePhoneNumber != "" {
		return resp.PhoneInfo.PurePhoneNumber, nil
	}
	if resp.PhoneInfo.PhoneNumber != "" {
		return resp.PhoneInfo.PhoneNumber, nil
	}
	return "", errors.New("wechat getuserphonenumber: empty phone number")
}

// accessToken 返回缓存的 access_token，过期或 force 时重新获取。
// 持锁期间完成 HTTP 调用，避免并发刷新风暴（本系统请求量小，可接受）。
func (c *Client) accessToken(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !force && c.token != "" && time.Now().Before(c.tokenExpiry) {
		return c.token, nil
	}

	body, err := json.Marshal(map[string]any{
		"grant_type":    "client_credential",
		"appid":         c.appID,
		"secret":        c.secret,
		"force_refresh": force,
	})
	if err != nil {
		return "", fmt.Errorf("encode stable_token request: %w", err)
	}

	var resp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := c.postJSON(ctx, "/cgi-bin/stable_token", body, &resp); err != nil {
		return "", fmt.Errorf("wechat stable_token: %w", err)
	}
	if resp.ErrCode != 0 {
		return "", fmt.Errorf("wechat stable_token: errcode=%d %s", resp.ErrCode, resp.ErrMsg)
	}
	if resp.AccessToken == "" {
		return "", errors.New("wechat stable_token: empty access_token")
	}

	ttl := time.Duration(resp.ExpiresIn)*time.Second - 5*time.Minute
	if ttl < time.Minute {
		ttl = time.Minute
	}
	c.token = resp.AccessToken
	c.tokenExpiry = time.Now().Add(ttl)
	return c.token, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	return c.do(req, out)
}

func (c *Client) postJSON(ctx context.Context, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", req.URL.Path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d from %s", resp.StatusCode, req.URL.Path)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
