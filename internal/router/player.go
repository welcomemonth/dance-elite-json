package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/service"
)

func RegisterPlayerRouters(api *gin.RouterGroup, apiService *service.APIService) {
	playerGroup := api.Group("/player")
	{
		playerGroup.GET("/list", func(ctx *gin.Context) {
			panic("unimplement")
		})
	}
}
