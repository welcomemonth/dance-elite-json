package utils

import (
	"crypto/rand"
	"math/big"
)

// Generate8CharID 生成 8 位 字母+数字 混合的随机 ID
func Generate8CharID() string {
	// 字符集：小写字母 + 数字。如果需要大写字母，可以加上 A-Z
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}
