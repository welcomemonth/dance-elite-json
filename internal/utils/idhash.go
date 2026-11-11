// Package utils 提供跨层复用的纯函数工具。
package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"unicode"
)

// 证件类型。与 players.id_type 的取值保持一致。
// 在线报名只接受 id_card（R7）；passport / hk_macau / other 由后台人工录入（T16）。
const (
	IDTypeIDCard   = "id_card"
	IDTypePassport = "passport"
	IDTypeHKMacau  = "hk_macau"
	IDTypeOther    = "other"
)

// NormalizeIDNumber 把证件号规范化成唯一的身份键形式：
//
//   - 所有证件类型：去掉全部空白字符（含首尾空格、全角空格、制表符）；
//   - 身份证：末位校验位 x 统一大写为 X；
//   - 护照等其他证件：统一大写。
//
// 同一号码的任何输入形式（含空格、大小写差异）必须得到相同的返回值，
// 否则同一人会被重复建档。当前 id_card 与 passport 的规则一致（去空白 +
// 大写），保留 idType 参数以便未来按证件类型扩展。
func NormalizeIDNumber(idType, raw string) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, raw)
	return strings.ToUpper(s)
}

// HashIDNumber 计算证件号的 HMAC-SHA256（十六进制小写字符串）。
// 入参先按 NormalizeIDNumber 规范化，因此 "110...x " 与 "110...X" 同值。
//
// key 必须是非空的独立服务端密钥（security.id_card_hmac_key）；
// 数据库只存哈希，永不存明文。
func HashIDNumber(key []byte, idType, raw string) string {
	return HashNormalizedIDNumber(key, NormalizeIDNumber(idType, raw))
}

// HashNormalizedIDNumber 对已规范化的证件号做 HMAC-SHA256。
// 供批量导入等已提前规范化的场景复用，避免重复规范化。
func HashNormalizedIDNumber(key []byte, normalized string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil))
}

// EqualIDHash 以常量时间比较两个证件号哈希，避免时序侧信道。
func EqualIDHash(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// MaskIDNumber 返回适合写日志的证件号掩码：保留前 3 位与后 4 位，
// 中间全部替换为 *。长度不足 8 位时整体打码。
func MaskIDNumber(raw string) string {
	r := []rune(NormalizeIDNumber("", raw))
	const head, tail = 3, 4
	if len(r) == 0 {
		return ""
	}
	if len(r) <= head+tail {
		return strings.Repeat("*", len(r))
	}
	return string(r[:head]) + strings.Repeat("*", len(r)-head-tail) + string(r[len(r)-tail:])
}
