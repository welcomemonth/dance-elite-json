package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
)

func (r *Router) registerRankRoutes(api *gin.RouterGroup) {
	rankGroup := api.Group("/rank")
	{
		rankGroup.GET("", r.getRankList)
	}
}

func (r *Router) getRankList(c *gin.Context) {
	group := c.Query("group")
	dance_type := c.Query("dance_type")

	// 2. 调用 Service（通过 r.apiService 访问，不用再传参）
	rankGroups, err := r.apiService.GetRankGroups(c.Request.Context())
	if err != nil {
		c.JSON(200, model.Response{
			Code:    1001,
			Message: "get rank failed",
			Data:    err.Error(),
		})
		return
	}

	// 根据group和dance_type获取真正的过滤
	filterStr := group
	switch dance_type {
	case "folk":
		filterStr += "-" + "民族民间舞"
	case "classial":
		filterStr += "-" + "古典舞"
	default:
		c.JSON(200, model.Response{
			Code:    1001,
			Message: "bad dance type",
			Data:    nil,
		})
		return
	}
	var rankGroup model.RankGroup
	logger.Infow("获取排行榜的请求参数：", "filter", filterStr)
	for _, rank := range rankGroups {
		if rank.Category == filterStr {
			rankGroup = *rank // 怎么进行深拷贝？
			break
		}
	}
	c.JSON(200, model.Response{
		Code:    0,
		Message: "success",
		Data:    rankGroup,
	})
	// 临时创建一个 具有直通排行榜和积分排行榜的内容
	// var
}
