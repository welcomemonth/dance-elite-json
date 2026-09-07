package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/model"
)

func (r *Router) registerConfigRoutes(api *gin.RouterGroup) {
	api.GET("/config", func(ctx *gin.Context) {
		cfg, err := r.apiService.GetConfig(ctx)
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
}
