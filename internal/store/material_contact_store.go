package store

import (
	"context"
	"database/sql"
	"fmt"
)

// MaterialContactStore 是资料催收联系记录（material_contacts 表）的数据访问层。
// 每次标记已联系追加一条，历史不可覆盖（API 文档 12.4 催收留痕）。
type MaterialContactStore struct{ db *DB }

// NewMaterialContactStore 构造资料催收联系记录 store。
func NewMaterialContactStore(db *DB) *MaterialContactStore {
	return &MaterialContactStore{db: db}
}

// CreateTx 在事务内追加一条联系记录。
func (s *MaterialContactStore) CreateTx(ctx context.Context, tx *sql.Tx, registrationID, createdBy int64, note *string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO material_contacts (registration_id, note, created_by)
		VALUES (?, ?, ?)`, registrationID, note, createdBy); err != nil {
		return fmt.Errorf("insert material contact of registration %d: %w", registrationID, err)
	}
	return nil
}
