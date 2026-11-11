package router

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/service"
)

// materialHandler 承载参赛资料更新接口（T13，API 文档 4.1）。
type materialHandler struct {
	svc *service.MaterialService
}

// registerMaterialRoutes 注册 PUT /api/v1/registrations/{registration_id}/materials。
// 需用户登录；按 user_id 限流 30 次/分钟（支持分次保存，重复提交安全）。
func registerMaterialRoutes(group *gin.RouterGroup, svc *service.MaterialService, signer *token.Signer) {
	h := &materialHandler{svc: svc}
	group.PUT("/registrations/:registration_id/materials",
		middleware.UserAuth(signer),
		middleware.RateLimitKeyed(30, time.Minute, userRateKey),
		h.update)
}

// update 解析 multipart（文本字段 + photo/audio 文件）并调用 service。
func (h *materialHandler) update(c *gin.Context) {
	registrationID, err := strconv.ParseInt(c.Param("registration_id"), 10, 64)
	if err != nil || registrationID <= 0 {
		httpx.AbortError(c, httpx.ErrBadRequest("报名 ID 不合法"))
		return
	}
	userID, ok := middleware.GetUserID(c)
	if !ok {
		httpx.AbortError(c, httpx.ErrUnauthorized("未登录或登录已过期"))
		return
	}

	// 请求体上限：音频 100MB + 证件照 10MB + multipart 开销，超限即拒绝，
	// 避免把超大请求完整读入内存/磁盘。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxMaterialRequestBytes)
	form, err := c.MultipartForm()
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.AbortError(c, httpx.ErrBadRequest("请求体过大：音频不能超过 100MB、证件照不能超过 10MB"))
			return
		}
		httpx.AbortError(c, httpx.ErrBadRequest("请求格式错误：需使用 multipart/form-data"))
		return
	}

	input := service.MaterialUpdateInput{
		ProgramName:    formValue(form, "program_name"),
		TeacherName:    formValue(form, "teacher_name"),
		ContactPhone:   formValue(form, "contact_phone"),
		ExpressAddress: formValue(form, "express_address"),
		WorkIntro:      formValue(form, "work_intro"),
		SpecialReq:     formValue(form, "special_req"),
	}

	var closeFiles []func()
	open := func(field string) (*service.MaterialFile, error) {
		file, closer, err := openFormFile(form, field)
		if closer != nil {
			closeFiles = append(closeFiles, closer)
		}
		return file, err
	}
	defer func() {
		for _, closeFn := range closeFiles {
			closeFn()
		}
	}()

	if input.Photo, err = open("photo"); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("读取证件照失败，请重试"))
		return
	}
	if input.Audio, err = open("audio"); err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("读取音频失败，请重试"))
		return
	}

	resp, err := h.svc.Update(c.Request.Context(), userID, registrationID, input)
	if err != nil {
		httpx.AbortError(c, err)
		return
	}
	httpx.OK(c, resp)
}
