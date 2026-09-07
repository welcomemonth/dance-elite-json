package router

import (
	"github.com/gin-gonic/gin"
)

func (r *Router) registerPlayerRoutes(api *gin.RouterGroup) {
	playerGroup := api.Group("/player")
	{
		playerGroup.GET("/list", func(ctx *gin.Context) {
			panic("unimplement")
		})
	}
}
