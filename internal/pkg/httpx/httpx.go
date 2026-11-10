// Package httpx 提供统一响应包装与全局错误码，对照 docs/优舞盟报名功能/API接口设计.md 0.3/0.4。
//
// 使用方式：handler 只做参数校验与调用 service，出错时
//
//	httpx.AbortError(c, httpx.ErrNotFound("赛事不存在"))
//	return
//
// HandleErrors 中间件统一把 c.Errors 渲染为 {"code", "message", "data": null}，
// 并按错误码映射 HTTP Status。service 返回的未知错误一律包装为 50001。
package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 全局错误码，与 API 文档 0.4 一致；报名域业务错误码（41001-41007）在 T8/T9 引入。
const (
	CodeOK              = 0
	CodeBadRequest      = 40001 // 参数错误 / 校验失败
	CodeUnauthorized    = 40101 // 未登录或 token 失效
	CodeForbidden       = 40301 // 无权限（角色不允许）
	CodeNotFound        = 40401 // 资源不存在
	CodeConflict        = 40901 // 业务冲突（重复报名、名额已满等）
	CodeTooManyRequests = 42901 // 触发限流
	CodeInternal        = 50001 // 服务器内部错误
)

// Response 统一响应包装。
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// BizError 用户可见的业务错误：Code 为业务错误码，Status 为 HTTP 状态。
// Message 直接返回给前端；内部细节放 Cause（只进日志，不返回）。
type BizError struct {
	Code    int
	Message string
	Status  int
	Cause   error
}

func (e *BizError) Error() string { return e.Message }

// Unwrap 暴露底层错误，供 errors.Is/As 与日志链使用。
func (e *BizError) Unwrap() error { return e.Cause }

func newBiz(status, code int, msg string, cause error) *BizError {
	return &BizError{Code: code, Message: msg, Status: status, Cause: cause}
}

// ErrBadRequest 参数错误 / 校验失败（40001 / HTTP 400）。
func ErrBadRequest(msg string) *BizError {
	return newBiz(http.StatusBadRequest, CodeBadRequest, msg, nil)
}

// ErrUnauthorized 未登录或 token 失效（40101 / HTTP 401）。
func ErrUnauthorized(msg string) *BizError {
	return newBiz(http.StatusUnauthorized, CodeUnauthorized, msg, nil)
}

// ErrForbidden 无权限（40301 / HTTP 403）。
func ErrForbidden(msg string) *BizError {
	return newBiz(http.StatusForbidden, CodeForbidden, msg, nil)
}

// ErrNotFound 资源不存在（40401 / HTTP 404）。
func ErrNotFound(msg string) *BizError {
	return newBiz(http.StatusNotFound, CodeNotFound, msg, nil)
}

// ErrConflict 业务冲突（40901 / HTTP 409）。
func ErrConflict(msg string) *BizError {
	return newBiz(http.StatusConflict, CodeConflict, msg, nil)
}

// ErrTooManyRequests 触发限流（42901 / HTTP 429）。
func ErrTooManyRequests(msg string) *BizError {
	return newBiz(http.StatusTooManyRequests, CodeTooManyRequests, msg, nil)
}

// ErrInternal 服务器内部错误（50001 / HTTP 500），cause 记录细节供日志。
func ErrInternal(cause error, msg string) *BizError {
	if msg == "" {
		msg = "服务器内部错误"
	}
	return newBiz(http.StatusInternalServerError, CodeInternal, msg, cause)
}

// Wrap 把任意 error 映射为 BizError：已是 BizError 原样返回，其余视为 50001。
func Wrap(err error) *BizError {
	if err == nil {
		return nil
	}
	var biz *BizError
	if errors.As(err, &biz) {
		return biz
	}
	return ErrInternal(err, "")
}

// OK 渲染成功响应：{"code": 0, "message": "success", "data": ...}。
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{Code: CodeOK, Message: "success", Data: data})
}

// AbortError 把错误挂到 c.Errors 并中断处理链，由 HandleErrors 统一渲染。
// 注意不能用 c.AbortWithError：gin v1.12 起它会立即写入 status，
// 导致 HandleErrors 因 Writer.Written() 而跳过渲染。
func AbortError(c *gin.Context, err error) {
	if err != nil {
		_ = c.Error(err)
	}
	c.Abort()
}

// HandleErrors 中间件：处理链结束后，如有挂起的错误则渲染统一响应。
// 已自行写入响应的处理链不重复写。
func HandleErrors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}
		biz := Wrap(c.Errors[0].Err)
		c.JSON(biz.Status, Response{Code: biz.Code, Message: biz.Message, Data: nil})
	}
}
