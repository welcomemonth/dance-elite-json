package store

import (
	"errors"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ErrNotFound 表示查询目标不存在。调用方用 errors.Is(err, store.ErrNotFound) 判断，
// 不依赖具体数据库错误（如 sql.ErrNoRows）。
var ErrNotFound = errors.New("store: record not found")

// ErrConflict 表示写入与当前数据状态冲突（如状态机条件流转未命中、
// 名额只能调大的条件 UPDATE 未通过）。调用方用 errors.Is 判断。
var ErrConflict = errors.New("store: state conflict")

// IsUniqueViolation 判断是否为 SQLite 唯一约束冲突（含部分唯一索引）。
// service 层据此把并发/重复写入映射为 40901，而非 50001。
func IsUniqueViolation(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
	}
	return false
}
