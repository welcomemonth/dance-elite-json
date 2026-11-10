package utils

import "time"

// isoLayout 是全库统一的时间格式：ISO 8601 本地时间，带时区偏移。
// 部署时区为 Asia/Shanghai，即 2027-01-01T10:00:00+08:00。
const isoLayout = "2006-01-02T15:04:05-07:00"

// NowISO 返回当前时间的统一格式字符串，供需要显式写入时间戳的 store/service 使用
// （其余 created_at/updated_at 由数据库默认值与触发器维护）。
func NowISO() string {
	return time.Now().Format(isoLayout)
}
