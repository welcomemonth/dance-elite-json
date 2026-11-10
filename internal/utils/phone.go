package utils

// MaskPhone 对手机号打码用于展示：保留前 3 位与后 4 位，如 138****5678。
// 长度不足以打码时返回固定掩码，避免泄漏短号码。
func MaskPhone(phone string) string {
	if len(phone) < 8 {
		return "****"
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}
