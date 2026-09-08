package router

import (
	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/model"
)

func (r *Router) registerInstitutionRoute(api *gin.RouterGroup) {
	group := api.Group("/institution")
	group.GET("/ranking", r.getInstitutionRanking)
}

func (r *Router) getInstitutionRanking(ctx *gin.Context) {
	institutionRank, err := r.apiService.GetInstitutionRank(ctx.Request.Context())
	if err != nil {
		ctx.JSON(200, model.Response{
			Code:    1002,
			Message: "get institution rank failed",
			Data:    err,
		})
		return
	}
	ctx.JSON(200, model.Response{
		Code:    0,
		Message: "success",
		Data:    institutionRank,
	})
	return
}
