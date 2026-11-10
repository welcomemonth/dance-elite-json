package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
)

const requestIDHeader = "X-Request-ID"
const requestIDKey = "request_id"

// RequestID 为每个请求注入/透传 X-Request-ID。
// 客户端未携带时生成 32 位十六进制随机 ID，写入响应头、gin.Context
// 以及请求 context（携带 request_id 字段的 logger），下游日志据此串联。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		c.Set(requestIDKey, id)
		c.Writer.Header().Set(requestIDHeader, id)

		// 挂 request-scoped logger：后续 AccessLog / Recovery / handler
		// 经 logger.FromCtx 取出的日志自动带 request_id，无需手动传字段。
		ctx := logger.Ctx(c.Request.Context(),
			logger.FromCtx(c.Request.Context()).With("request_id", id))
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// GetRequestID 从 gin.Context 取出请求 ID（访问日志使用）。
func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get(requestIDKey); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// newRequestID 生成 16 字节随机数的十六进制字符串。
func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 不可恢复，降级为全零（理论上不发生）
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b)
}
