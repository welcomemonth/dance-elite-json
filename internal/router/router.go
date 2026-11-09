package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/audio"
	"github.com/meetmonth/dance-elite/internal/service"
)

type Router struct {
	engine       *gin.Engine
	apiService   *service.APIService
	audioManager *audio.Manager
}

// New 构建并返回一个已注册所有路由的 gin.Engine。
//
// 返回 engine 而非空值，是为了让测试能用 httptest.NewRecorder
// 直接对路由做接口级测试（配合注入 fake service 即可，无需启动真实端口）。
func New(apiService *service.APIService, audioManager *audio.Manager) *gin.Engine {
	r := &Router{
		engine:       gin.Default(),
		apiService:   apiService,
		audioManager: audioManager,
	}

	// engine.Use(middleware.RequestID(), middleware.GinLogger(), middleware.CORS())

	// 设置最大文件上传大小
	r.engine.MaxMultipartMemory = 50 << 20

	// 静态文件服务
	r.engine.Static("/static", "./uploads")

	// 注册 contest 路由（/contest 页面与 /api/contest/* API）
	r.registerContestRoutes()

	// API 路由
	api := r.engine.Group("/api/v1")

	// 健康检查
	api.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	// 注册各模块路由（变成方法调用）
	r.registerConfigRoutes(api)
	r.registerRankRoutes(api)
	r.registerDancerRoutes(api)
	r.registerInstitutionRoute(api)
	return r.engine
}
