package utils

import "time"

// isoLayout 是全库统一的时间格式：ISO 8601 本地时间，带时区偏移。
// 部署时区为 Asia/Shanghai，即 2027-01-01T10:00:00+08:00。
const isoLayout = "2006-01-02T15:04:05-07:00"

// dateLayout 是日期字段（event_date、赛季起止日期等）的统一格式。
const dateLayout = "2006-01-02"

// ChinaLoc 是部署目标时区（Asia/Shanghai，固定 +8，无夏令时）。
// 全库时间字符串统一以该时区偏移存储（...+08:00），保证 DB 侧字符串比较
// （时间线 CHECK 约束）与语义一致；部署容器须设 TZ=Asia/Shanghai。
var ChinaLoc = time.FixedZone("Asia/Shanghai", 8*3600)

// NowISO 返回当前时间的统一格式字符串，供需要显式写入时间戳的 store/service 使用
// （其余 created_at/updated_at 由数据库默认值与触发器维护）。
func NowISO() string {
	return time.Now().Format(isoLayout)
}

// FormatISO 把任意时间转换为全库统一格式（Asia/Shanghai 时区 + 偏移）。
// 用于外部时间（如微信支付 success_time）入库前的归一化。
func FormatISO(t time.Time) string {
	return t.In(ChinaLoc).Format(isoLayout)
}

// ParseISO 解析统一格式的时间字符串（ISO 8601 带时区偏移）。
func ParseISO(s string) (time.Time, error) {
	return time.Parse(isoLayout, s)
}

// ParseDate 解析统一格式的日期字符串（YYYY-MM-DD）。
func ParseDate(s string) (time.Time, error) {
	return time.Parse(dateLayout, s)
}
