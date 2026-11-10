package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
)

// AccessLog 记录每个请求的访问日志（zap），request_id 已由 RequestID
// 中间件注入到 context logger，这里直接取用。panic 由 Recovery 记录。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		l := logger.FromCtx(c.Request.Context())
		status := c.Writer.Status()
		fields := []interface{}{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
			"errors", c.Errors.ByType(gin.ErrorTypePrivate).String(),
		}
		switch {
		case status >= 500:
			l.Errorw("http access", fields...)
		case status >= 400:
			l.Warnw("http access", fields...)
		default:
			l.Infow("http access", fields...)
		}
	}
}
