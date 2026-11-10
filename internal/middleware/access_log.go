package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
)

// AccessLog 记录每个请求的访问日志（zap），字段对齐现有 logger 输出。
// 仅在处理链正常返回时记录；panic 由 Recovery 中间件记录。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)

		status := c.Writer.Status()
		msg := "http access"
		fields := []interface{}{
			"request_id", GetRequestID(c),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"client_ip", c.ClientIP(),
			"errors", c.Errors.ByType(gin.ErrorTypePrivate).String(),
		}
		switch {
		case status >= 500:
			logger.Errorw(msg, fields...)
		case status >= 400:
			logger.Warnw(msg, fields...)
		default:
			logger.Infow(msg, fields...)
		}
	}
}
