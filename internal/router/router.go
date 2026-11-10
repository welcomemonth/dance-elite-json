package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
)

// New 构建并返回一个已注册全局中间件的 gin.Engine。
//
// 业务路由（小程序 /api/v1、后台 /api/v1/admin、H5 /h5）将在 T2+ 按模块注册；
// 依赖通过参数注入，便于 httptest 直接对接口做测试。
//
// 返回 engine 而非空值，是为了让测试能用 httptest.NewRecorder
// 直接对路由做接口级测试（无需启动真实端口）。
func New() *gin.Engine {
	engine := gin.New()

	// 全局中间件：Recovery -> RequestID -> AccessLog -> 统一错误渲染
	engine.Use(middleware.Recovery(), middleware.RequestID(), middleware.AccessLog(), httpx.HandleErrors())

	api := engine.Group("/api/v1")

	// 健康检查
	api.GET("/ping", func(c *gin.Context) {
		httpx.OK(c, gin.H{"message": "pong"})
	})

	return engine
}
