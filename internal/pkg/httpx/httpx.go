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

// 全局错误码，与 API 文档 0.4/0.5 一致；41012 见下方报名域定义（API 文档 0.5）。
const (
	CodeOK              = 0
	CodeBadRequest      = 40001 // 参数错误 / 校验失败
	CodeUnauthorized    = 40101 // 未登录或 token 失效
	CodeAccountNotFound = 40102 // 后台登录：账号不存在
	CodeWrongPassword   = 40103 // 后台登录：密码错误
	CodeAccountLocked   = 40104 // 后台登录：账号已锁定（连续密码错误）
	CodeForbidden       = 40301 // 无权限（角色不允许）
	CodeAccountDisabled = 40302 // 后台登录：账号已停用
	CodeNotFound        = 40401 // 资源不存在
	CodeConflict        = 40901 // 业务冲突（重复报名、名额已满等）
	CodeTooManyRequests = 42901 // 触发限流
	CodeInternal        = 50001 // 服务器内部错误

	// 报名域业务错误码（API 文档 0.5）：41001/41002 证件号与年龄校验（T8 引入），
	// 41003-41007 报名校验（T9 引入），41012 赛事状态不允许该操作（后台赛事管理，T6 引入）。
	CodeInvalidIDNumber      = 41001 // 身份证号格式不合法
	CodeAgeNotAllowed        = 41002 // 不符合赛事年龄要求
	CodeLevelAlreadyTaken    = 41003 // 该赛事已有有效报名（一人一组别，可新增其他舞种）
	CodeDanceTypeDuplicate   = 41004 // 该舞种已报名（请勿重复提交）
	CodeQuotaFull            = 41005 // 名额已满（引导候补）
	CodeNotInRegPeriod       = 41006 // 赛事不在报名期
	CodeGroupDisabled        = 41007 // 组别已关闭
	CodeEventStateNotAllowed = 41012 // 赛事状态不允许该操作（API 文档 10.3/10.4/10.5）
)

// Response 统一响应包装。
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// BizError 用户可见的业务错误：Code 为业务错误码，Status 为 HTTP 状态。
// Message 直接返回给前端；内部细节放 Cause（只进日志，不返回）。
// Data 为可选的业务数据载荷（如 41005 返回已满组别信息引导候补），
// 随错误响应一并返回。
type BizError struct {
	Code    int
	Message string
	Status  int
	Data    interface{}
	Cause   error
}

func (e *BizError) Error() string { return e.Message }

// Unwrap 暴露底层错误，供 errors.Is/As 与日志链使用。
func (e *BizError) Unwrap() error { return e.Cause }

func newBiz(status, code int, msg string, cause error) *BizError {
	return &BizError{Code: code, Message: msg, Status: status, Cause: cause}
}

// newBizData 构造带业务数据载荷的错误（如 41005 附已满组别信息）。
func newBizData(status, code int, msg string, data interface{}, cause error) *BizError {
	return &BizError{Code: code, Message: msg, Status: status, Data: data, Cause: cause}
}

// ErrBadRequest 参数错误 / 校验失败（40001 / HTTP 400）。
func ErrBadRequest(msg string) *BizError {
	return newBiz(http.StatusBadRequest, CodeBadRequest, msg, nil)
}

// ErrUnauthorized 未登录或 token 失效（40101 / HTTP 401）。
func ErrUnauthorized(msg string) *BizError {
	return newBiz(http.StatusUnauthorized, CodeUnauthorized, msg, nil)
}

// ErrAccountNotFound 后台登录：账号不存在（40102 / HTTP 401）。
func ErrAccountNotFound(msg string) *BizError {
	return newBiz(http.StatusUnauthorized, CodeAccountNotFound, msg, nil)
}

// ErrWrongPassword 后台登录：密码错误（40103 / HTTP 401）。
func ErrWrongPassword(msg string) *BizError {
	return newBiz(http.StatusUnauthorized, CodeWrongPassword, msg, nil)
}

// ErrAccountLocked 后台登录：账号已锁定（40104 / HTTP 401）。
func ErrAccountLocked(msg string) *BizError {
	return newBiz(http.StatusUnauthorized, CodeAccountLocked, msg, nil)
}

// ErrForbidden 无权限（40301 / HTTP 403）。
func ErrForbidden(msg string) *BizError {
	return newBiz(http.StatusForbidden, CodeForbidden, msg, nil)
}

// ErrAccountDisabled 账号已停用（40302 / HTTP 403，后台登录 API 文档 8.1）。
func ErrAccountDisabled(msg string) *BizError {
	return newBiz(http.StatusForbidden, CodeAccountDisabled, msg, nil)
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

// ErrInvalidIDNumber 身份证号格式不合法（41001 / HTTP 400，API 文档 3.1）。
func ErrInvalidIDNumber(msg string) *BizError {
	return newBiz(http.StatusBadRequest, CodeInvalidIDNumber, msg, nil)
}

// ErrAgeNotAllowed 不符合赛事年龄要求（41002 / HTTP 400，API 文档 3.1）。
// msg 中附匹配到的组别出生年份区间，供小程序提示。
func ErrAgeNotAllowed(msg string) *BizError {
	return newBiz(http.StatusBadRequest, CodeAgeNotAllowed, msg, nil)
}

// ErrLevelAlreadyTaken 该赛事已有有效报名，一人一组别（41003 / HTTP 400，API 文档 3.1）。
// msg 描述已有报名（组别+舞种），提示可为该组别新增另一舞种。
func ErrLevelAlreadyTaken(msg string) *BizError {
	return newBiz(http.StatusBadRequest, CodeLevelAlreadyTaken, msg, nil)
}

// ErrDanceTypeDuplicate 该舞种已报名，请勿重复提交（41004 / HTTP 400，API 文档 3.1）。
func ErrDanceTypeDuplicate(msg string) *BizError {
	return newBiz(http.StatusBadRequest, CodeDanceTypeDuplicate, msg, nil)
}

// ErrQuotaFull 名额已满（41005 / HTTP 400，API 文档 3.1）。data 为已满组别信息
// （group_id/level/dance_type），前端据此引导候补表单。
func ErrQuotaFull(msg string, data interface{}) *BizError {
	return newBizData(http.StatusBadRequest, CodeQuotaFull, msg, data, nil)
}

// ErrNotInRegPeriod 赛事不在报名期（41006 / HTTP 400，API 文档 3.1）。
func ErrNotInRegPeriod(msg string) *BizError {
	return newBiz(http.StatusBadRequest, CodeNotInRegPeriod, msg, nil)
}

// ErrGroupDisabled 组别已关闭（41007 / HTTP 400，API 文档 3.1）。
func ErrGroupDisabled(msg string) *BizError {
	return newBiz(http.StatusBadRequest, CodeGroupDisabled, msg, nil)
}

// ErrEventStateNotAllowed 赛事状态不允许该操作（41012 / HTTP 400，API 文档 10.3）。
func ErrEventStateNotAllowed(msg string) *BizError {
	return newBiz(http.StatusBadRequest, CodeEventStateNotAllowed, msg, nil)
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
		c.JSON(biz.Status, Response{Code: biz.Code, Message: biz.Message, Data: biz.Data})
	}
}
