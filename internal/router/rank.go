package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/service"
)

func RegisterRankRouters(api *gin.RouterGroup, apiService *service.APIService) {
	rankGroup := api.Group("/rank")
	{
		rankGroup.GET("")
	}
}
