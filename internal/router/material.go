package router

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/service"
)

// materialHandler 承载参赛资料更新接口（T13，API 文档 4.1–4.3）。
type materialHandler struct {
	svc *service.MaterialService
}

// registerMaterialRoutes 注册文本、证件照、音频三个独立更新接口。
// 均需用户登录并按 user_id 限流：文本 30 次/分钟，证件照 20 次/分钟，音频 10 次/分钟。
func registerMaterialRoutes(group *gin.RouterGroup, svc *service.MaterialService, signer *token.Signer) {
	h := &materialHandler{svc: svc}
	auth := middleware.UserAuth(signer)
	group.PUT("/registrations/:registration_id/materials",
		auth, middleware.RateLimitKeyed(30, time.Minute, userRateKey), h.updateText)
	group.PUT("/registrations/:registration_id/materials/photo",
		auth, middleware.RateLimitKeyed(20, time.Minute, userRateKey), h.updatePhoto)
	group.PUT("/registrations/:registration_id/materials/audio",
		auth, middleware.RateLimitKeyed(10, time.Minute, userRateKey), h.updateAudio)
}

// materialTextRequest 是文本资料更新请求体（nil = 未提交，不修改；空串 = 清空）。
type materialTextRequest struct {
	ProgramName    *string `json:"program_name"`
	TeacherName    *string `json:"teacher_name"`
	ContactPhone   *string `json:"contact_phone"`
	ExpressAddress *string `json:"express_address"`
	WorkIntro      *string `json:"work_intro"`
	SpecialReq     *string `json:"special_req"`
}

// updateText 更新 6 项文本资料（JSON）。
func (h *materialHandler) updateText(c *gin.Context) {
	registrationID, userID, ok := materialRequestIDs(c)
	if !ok {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxTextRequestBytes)
	var req materialTextRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.AbortError(c, httpx.ErrBadRequest("请求体过大"))
			return
		}
		httpx.AbortError(c, httpx.ErrBadRequest("请求体格式错误：需为 JSON"))
		return
	}

	resp, err := h.svc.UpdateText(c.Request.Context(), userID, registrationID, service.MaterialTextInput{
		ProgramName:    req.ProgramName,
		TeacherName:    req.TeacherName,
		ContactPhone:   req.ContactPhone,
		ExpressAddress: req.ExpressAddress,
		WorkIntro:      req.WorkIntro,
		SpecialReq:     req.SpecialReq,
	})
	h.writeResult(c, resp, err)
}

// updatePhoto 上传/替换证件照（multipart，字段名 photo）。
func (h *materialHandler) updatePhoto(c *gin.Context) {
	registrationID, userID, ok := materialRequestIDs(c)
	if !ok {
		return
	}
	file, closeFn, ok := readUploadedFile(c, "photo", "证件照",
		"证件照不能超过 10MB", service.MaxPhotoRequestBytes)
	if !ok {
		return
	}
	defer closeFn()

	resp, err := h.svc.UpdatePhoto(c.Request.Context(), userID, registrationID, file)
	h.writeResult(c, resp, err)
}

// updateAudio 上传/替换舞蹈音频（multipart，字段名 audio）。
func (h *materialHandler) updateAudio(c *gin.Context) {
	registrationID, userID, ok := materialRequestIDs(c)
	if !ok {
		return
	}
	file, closeFn, ok := readUploadedFile(c, "audio", "音频",
		"音频不能超过 100MB", service.MaxAudioRequestBytes)
	if !ok {
		return
	}
	defer closeFn()

	resp, err := h.svc.UpdateAudio(c.Request.Context(), userID, registrationID, file)
	h.writeResult(c, resp, err)
}

func (h *materialHandler) writeResult(c *gin.Context, resp *model.UpdateMaterialsResponse, err error) {
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}

// materialRequestIDs 解析 path 中的报名 ID 与登录用户 ID；失败时已写响应。
func materialRequestIDs(c *gin.Context) (registrationID, userID int64, ok bool) {
	registrationID, err := strconv.ParseInt(c.Param("registration_id"), 10, 64)
	if err != nil || registrationID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("报名 ID 不合法"))
		return 0, 0, false
	}
	userID, ok = middleware.GetUserID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return 0, 0, false
	}
	return registrationID, userID, true
}
