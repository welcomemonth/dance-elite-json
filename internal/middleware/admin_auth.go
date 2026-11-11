package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
)

const (
	adminIDKey   = "admin_id"
	adminRoleKey = "admin_role"
)

// AdminAuth 校验后台管理员 Bearer token，解析成功后把 admin_id/role 写入
// gin.Context；缺失或失效一律返回 40101。
//
// 与 UserAuth 使用同一签名器但校验 subject=admin：小程序用户 token
// 不能冒用后台接口。须在 RequireRole 之前注册。
func AdminAuth(signer *token.Signer) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
			return
		}
		claims, err := signer.ParseAdmin(raw)
		if err != nil {
			httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
			return
		}
		c.Set(adminIDKey, claims.AdminID)
		c.Set(adminRoleKey, claims.Role)
		c.Next()
	}
}

// RequireRole 角色守卫：要求 AdminAuth 写入的角色属于给定集合，否则 40301。
// 用法：admin.POST("/events", AdminAuth(signer), RequireRole(model.RoleAdmin, model.RoleSuperAdmin), h.create)。
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role, ok := GetAdminRole(c)
		if !ok {
			httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
			return
		}
		if _, ok := allowed[role]; !ok {
			httpx.AbortError(c, httpx.ErrForbidden("当前角色无权执行此操作"))
			return
		}
		c.Next()
	}
}

// GetAdminID 取出 AdminAuth 写入的管理员 ID。
func GetAdminID(c *gin.Context) (int64, bool) {
	v, ok := c.Get(adminIDKey)
	if !ok {
		return 0, false
	}
	id, ok := v.(int64)
	return id, ok
}

// GetAdminRole 取出 AdminAuth 写入的管理员角色。
func GetAdminRole(c *gin.Context) (string, bool) {
	v, ok := c.Get(adminRoleKey)
	if !ok {
		return "", false
	}
	role, ok := v.(string)
	return role, ok
}
