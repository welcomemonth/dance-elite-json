package router

import (
	"errors"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/service"
)

// readUploadedFile 解析 multipart 表单并打开指定字段的文件。
// 解析或打开失败时直接写错误响应并返回 ok=false；成功时调用方负责 closeFn。
func readUploadedFile(c *gin.Context, field, label, tooLargeMessage string, maxBytes int64) (
	*service.MaterialFile, func(), bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	form, err := c.MultipartForm()
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.AbortError(c, httpx.ErrBadRequest(tooLargeMessage))
			return nil, nil, false
		}
		httpx.AbortError(c, httpx.ErrBadRequest("请求格式错误：需使用 multipart/form-data"))
		return nil, nil, false
	}

	file, closer, err := openFormFile(form, field)
	if err != nil {
		httpx.AbortError(c, httpx.ErrBadRequest("读取上传文件失败，请重试"))
		return nil, nil, false
	}
	if file == nil {
		httpx.AbortError(c, httpx.ErrBadRequest("请选择"+label+"文件"))
		return nil, nil, false
	}
	return file, closer, true
}

// openFormFile 打开 multipart 中的首个文件；字段未提交时返回 (nil, nil, nil)。
func openFormFile(form *multipart.Form, field string) (*service.MaterialFile, func(), error) {
	files := form.File[field]
	if len(files) == 0 {
		return nil, nil, nil
	}
	fh := files[0]
	f, err := fh.Open()
	if err != nil {
		return nil, nil, err
	}
	return &service.MaterialFile{Filename: fh.Filename, Size: fh.Size, Content: f},
		func() { _ = f.Close() }, nil
}
