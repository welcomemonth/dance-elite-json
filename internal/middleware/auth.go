package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
)

const (
	userIDKey = "user_id"
	openIDKey = "openid"
)

// UserAuth 校验小程序用户 Bearer token，解析成功后把 user_id/openid 写入
// gin.Context；缺失或失效一律返回 40101。
func UserAuth(signer *token.Signer) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
			return
		}
		claims, err := signer.ParseUser(raw)
		if err != nil {
			httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
			return
		}
		c.Set(userIDKey, claims.UserID)
		c.Set(openIDKey, claims.OpenID)
		c.Next()
	}
}

// GetUserID 取出 UserAuth 写入的用户 ID。
func GetUserID(c *gin.Context) (int64, bool) {
	v, ok := c.Get(userIDKey)
	if !ok {
		return 0, false
	}
	id, ok := v.(int64)
	return id, ok
}

// GetOpenID 取出 UserAuth 写入的微信 openid。
func GetOpenID(c *gin.Context) (string, bool) {
	v, ok := c.Get(openIDKey)
	if !ok {
		return "", false
	}
	openid, ok := v.(string)
	return openid, ok
}

// bearerToken 从 Authorization 头解析 Bearer token（scheme 大小写不敏感）。
func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	raw := strings.TrimSpace(header[len(prefix):])
	return raw, raw != ""
}
