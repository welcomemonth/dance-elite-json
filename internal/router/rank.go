package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/model"
)

func (r *Router) registerRankRoutes(api *gin.RouterGroup) {
	rankGroup := api.Group("/ranking")
	{
		rankGroup.GET("", r.getRanking)
	}
}

func (r *Router) getRanking(c *gin.Context) {
	level := c.Query("level")
	dance_type := c.Query("dance_type")

	if level == "" || dance_type == "" {
		c.JSON(200, model.Response{
			Code:    1001,
			Message: "bad dance type or group",
			Data:    nil,
		})
		return
	}
	// 2. 调用 Service（通过 r.apiService 访问，不用再传参）
	rankingData, err := r.apiService.GetRankingDatas(c.Request.Context())
	if err != nil {
		c.JSON(200, model.Response{
			Code:    1001,
			Message: "get rank failed",
			Data:    err.Error(),
		})
		return
	}

	var board *model.Board
	for _, b := range rankingData.Boards {
		if b.DanceType == dance_type && b.Level == level {
			board = &b
		}
	}
	if board == nil {
		c.JSON(400, model.Response{
			Code:    1002,
			Message: "Can't find ranking",
			Data:    nil, // 返回 直通 和 积分 榜单
		})
		return
	}
	c.JSON(200, model.Response{
		Code:    0,
		Message: "success",
		Data:    board, // 返回 直通 和 积分 榜单
	})
}
