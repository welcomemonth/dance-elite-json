package store

import "errors"

// ErrNotFound 表示查询目标不存在。调用方用 errors.Is(err, store.ErrNotFound) 判断，
// 不依赖具体数据库错误（如 sql.ErrNoRows）。
var ErrNotFound = errors.New("store: record not found")
