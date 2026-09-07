package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/service"
)

type RouterGroup struct {
	engine     *gin.Engine
	apiService *service.APIService
}

// New 构建并返回一个已注册所有路由的 gin.Engine。
//
// 返回 engine 而非空值，是为了让测试能用 httptest.NewRecorder
// 直接对路由做接口级测试（配合注入 fake service 即可，无需启动真实端口）。
func New(apiService *service.APIService) *gin.Engine {
	engine := gin.Default()

	// engine.Use(middleware.RequestID(), middleware.GinLogger(), middleware.CORS())

	// 设置最大文件上传大小
	engine.MaxMultipartMemory = 50 << 20 // 50 MB

	// 静态文件服务
	engine.Static("/uploads", "./uploads")

	// API 路由
	api := engine.Group("/api/v1")

	// 健康检查
	api.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	api.GET("/config", func(ctx *gin.Context) {
		cfg, err := apiService.GetConfig(ctx)
		if err != nil {
			ctx.JSON(200, model.Response{
				Code:    1001,
				Message: "get config failed",
				Data:    err.Error(),
			})
			return
		}
		ctx.JSON(200, model.Response{
			Code:    0,
			Message: "success",
			Data:    cfg,
		})
	})
	RegisterPlayerRouters(api, apiService)
	RegisterRankRouters(api, apiService)

	return engine
}
