package audio

// naturalLess 实现文件名自然排序：数字段按数值比较，非数字段按字节序比较。
// 例如 "U13-A2" < "U13-A10"，而普通字符串序是 "U13-A10" < "U13-A2"。
func naturalLess(a, b string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		da, db := isDigit(ca), isDigit(cb)

		switch {
		case da && db:
			// 两侧都是数字段：按数值比较，数值相同时较短（前导零少）的在前
			si, sj := i, j
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			na, nb := a[si:i], b[sj:j]
			if c := cmpNumeric(na, nb); c != 0 {
				return c < 0
			}
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
		case !da && !db:
			// 两侧都是非数字：逐字节比较
			if ca != cb {
				return ca < cb
			}
			i++
			j++
		default:
			// 数字与非数字交界：数字排在非数字之前
			return da
		}
	}
	// 一方已耗尽，较短者在前
	return len(a)-i < len(b)-j
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// cmpNumeric 比较两个数字串（可能含前导零）的数值大小。
func cmpNumeric(a, b string) int {
	ta, tb := trimLeadingZeros(a), trimLeadingZeros(b)
	if len(ta) != len(tb) {
		if len(ta) < len(tb) {
			return -1
		}
		return 1
	}
	switch {
	case ta < tb:
		return -1
	case ta > tb:
		return 1
	default:
		return 0
	}
}

// trimLeadingZeros 去除前导零，但至少保留一位。
func trimLeadingZeros(s string) string {
	i := 0
	for i < len(s)-1 && s[i] == '0' {
		i++
	}
	return s[i:]
}
