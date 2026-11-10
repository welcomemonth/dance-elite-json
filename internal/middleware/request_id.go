package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

const requestIDHeader = "X-Request-ID"
const requestIDKey = "request_id"

// RequestID 为每个请求注入/透传 X-Request-ID。
// 客户端未携带时生成 32 位十六进制随机 ID，写入响应头与 gin.Context，
// 访问日志与错误日志据此串联一次请求的全部记录。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		c.Set(requestIDKey, id)
		c.Writer.Header().Set(requestIDHeader, id)
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
