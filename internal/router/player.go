package router

import "github.com/gin-gonic/gin"

func RegisterPlayerRouters(api *gin.RouterGroup) {
	playerGroup := api.Group("/player")
	{
		playerGroup.GET("/list", func(ctx *gin.Context) {
			panic("unimplement")
		})
	}
}
