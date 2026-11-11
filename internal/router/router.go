package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/service"
	"github.com/meetmonth/dance-elite/internal/store"
)

// Deps 是路由层所需的全部外部依赖，由组合根（main）一次性注入。
// 业务模块（T2+）落地后在此追加 service 字段，例如：
//
//	Events        *service.EventService
//	Registrations *service.RegistrationService
//
// 测试时构造 Deps 的 fake 实现即可用 httptest 对接口做测试。
type Deps struct {
	Stores    *store.Store
	Auth      *service.AuthService
	AdminAuth *service.AdminAuthService
	Seasons   *service.SeasonService
	Events    *service.EventService
	Tokens    *token.Signer
}

// New 构建并返回一个已注册全局中间件的 gin.Engine。
//
// 业务路由（小程序 /api/v1、后台 /api/v1/admin、H5 /h5）将在 T2+ 按模块注册，
// 各模块 handler 只接收自己依赖的 service（依赖从 Deps 中取出后按组传参，
// 如 registerAdminRoutes(api.Group("/admin"), deps.Events, deps.Auth)）。
//
// 返回 engine 而非空值，是为了让测试能用 httptest.NewRecorder
// 直接对路由做接口级测试（无需启动真实端口）。
func New(deps Deps) *gin.Engine {
	engine := gin.New()

	// 全局中间件：Recovery -> RequestID -> AccessLog -> 统一错误渲染
	engine.Use(middleware.Recovery(), middleware.RequestID(), middleware.AccessLog(), httpx.HandleErrors())

	api := engine.Group("/api/v1")

	// 小程序端认证（T4）
	registerAuthRoutes(api, deps.Auth, deps.Tokens)

	// 后台端认证（T5）
	registerAdminAuthRoutes(api, deps.AdminAuth, deps.Tokens)

	// 后台端业务（T6+）：/api/v1/admin 下全部需要 admin token。
	if deps.Tokens != nil && deps.Seasons != nil && deps.Events != nil {
		admin := api.Group("/admin", middleware.AdminAuth(deps.Tokens))
		registerAdminSeasonRoutes(admin, deps.Seasons)
		registerAdminEventRoutes(admin, deps.Events)
	}

	// 健康检查
	api.GET("/ping", func(c *gin.Context) {
		httpx.OK(c, gin.H{"message": "pong"})
	})

	return engine
}
