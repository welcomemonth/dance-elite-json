package logger

import (
	"regexp"

	"github.com/meetmonth/dance-elite/internal/utils"
)

// idNumberPattern 匹配 18 位居民身份证号（末位可为 X），用于在自由文本
// （错误信息、回调报文）进入日志前打码。
var idNumberPattern = regexp.MustCompile(`[0-9]{17}[0-9Xx]`)

// RedactText 把文本中出现的身份证号替换为掩码（保留前 3 位与后 4 位），
// 用于访问日志、错误日志等可能夹带用户输入的字段。日志中绝不出现证件号明文。
func RedactText(s string) string {
	return idNumberPattern.ReplaceAllStringFunc(s, utils.MaskIDNumber)
}

// RedactIDNumber 对单个证件号字段打码；空值原样返回，便于在结构化日志中使用。
func RedactIDNumber(s string) string {
	if s == "" {
		return ""
	}
	return utils.MaskIDNumber(s)
}
