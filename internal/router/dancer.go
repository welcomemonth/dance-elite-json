package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/model"
)

func (r *Router) registerDancerRoutes(api *gin.RouterGroup) {
	DancerGroup := api.Group("/dancer")
	{
		DancerGroup.GET(":id", r.getDancerById)
	}
}

func (r *Router) getDancerById(ctx *gin.Context) {
	// 1、获取选手id值
	idStr := ctx.Param("id")
	// // 打印或返回测试
	// ctx.JSON(200, gin.H{
	// 	"message": "获取到的ID是: " + idStr,
	// })
	// 2、从服务中查询这个dancer  只需要id即可，每个榜单是有独立id的
	dancer, err := r.apiService.GetDancerById(ctx.Request.Context(), idStr)
	if err != nil {
		ctx.JSON(200, model.Response{
			Code:    1003,
			Message: "get dancer failed",
			Data:    err,
		})
		return
	}
	ctx.JSON(200, model.Response{
		Code:    0,
		Message: "success",
		Data:    dancer,
	})
}
