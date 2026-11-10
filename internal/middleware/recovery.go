package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
)

// Recovery 捕获处理链中的 panic：记录堆栈与请求 ID，返回统一 50001 响应。
// 替代 gin.Default() 内置的 Recovery，输出走 zap 并复用统一响应包装。
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		err := fmt.Errorf("panic: %v\n%s", recovered, debug.Stack())
		logger.FromCtx(c.Request.Context()).Errorw("panic recovered",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"error", err,
		)
		if c.Writer.Written() {
			return
		}
		c.JSON(http.StatusInternalServerError, httpx.Response{
			Code:    httpx.CodeInternal,
			Message: "服务器内部错误",
			Data:    nil,
		})
	})
}
