package utils

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidIDCard 表示身份证号未通过合法性校验（长度、字符、校验码或出生日期）。
// 错误信息中不包含证件号本身，防止明文进入日志。
var ErrInvalidIDCard = errors.New("invalid id card number")

// GB 11643-1999：18 位身份证第 1-17 位加权因子与校验码表。
var idCardWeights = [...]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}

const idCardCheckCodes = "10X98765432"

// ParseIDCard 校验 18 位大陆身份证并解析档案字段。
//
// 校验项：规范化（去空白 + 末位 x 大写）后长度必须为 18；前 17 位为数字，
// 末位为数字或 X；第 7-14 位构成真实存在且不在未来的出生日期；
// 加权校验码（GB 11643-1999）必须匹配。
//
// 返回规范化后的号码、出生日期（YYYY-MM-DD）与性别（male/female，由第 17 位
// 顺序码奇偶判定）。任一校验失败返回 ErrInvalidIDCard。
func ParseIDCard(raw string) (normalized, birthDate, gender string, err error) {
	normalized = NormalizeIDNumber(IDTypeIDCard, raw)
	if len(normalized) != 18 {
		return "", "", "", fmt.Errorf("%w: length must be 18", ErrInvalidIDCard)
	}
	for i := range 17 {
		if normalized[i] < '0' || normalized[i] > '9' {
			return "", "", "", fmt.Errorf("%w: first 17 chars must be digits", ErrInvalidIDCard)
		}
	}
	last := normalized[17]
	if last != 'X' && (last < '0' || last > '9') {
		return "", "", "", fmt.Errorf("%w: check char must be digit or X", ErrInvalidIDCard)
	}

	// 出生日期：第 7-14 位 YYYYMMDD。
	birth, err := time.Parse("20060102", normalized[6:14])
	if err != nil {
		return "", "", "", fmt.Errorf("%w: bad birth date segment", ErrInvalidIDCard)
	}
	if birth.After(time.Now()) {
		return "", "", "", fmt.Errorf("%w: birth date in future", ErrInvalidIDCard)
	}
	birthDate = birth.Format("2006-01-02")

	// 加权校验码。
	sum := 0
	for i := range 17 {
		sum += int(normalized[i]-'0') * idCardWeights[i]
	}
	if idCardCheckCodes[sum%11] != last {
		return "", "", "", fmt.Errorf("%w: check code mismatch", ErrInvalidIDCard)
	}

	// 性别：第 17 位顺序码奇数为男、偶数为女。
	if (normalized[16]-'0')%2 == 1 {
		gender = "male"
	} else {
		gender = "female"
	}
	return normalized, birthDate, gender, nil
}
