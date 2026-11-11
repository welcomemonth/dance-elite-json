package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SettingsStore 是系统键值配置（settings 表）的数据访问层。
// 值均为 JSON 文本（模板/规则/文案），写入只发生在迁移种子与后台内容管理（后续任务）。
type SettingsStore struct{ db *DB }

// NewSettingsStore 构造系统配置 store。
func NewSettingsStore(db *DB) *SettingsStore { return &SettingsStore{db: db} }

// Get 返回指定 key 的配置值原文（JSON 文本）。不存在返回 ErrNotFound。
func (s *SettingsStore) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.sql.QueryRowContext(ctx, `SELECT value FROM settings WHERE "key" = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("query setting %q: %w", key, err)
	}
	return value, nil
}
