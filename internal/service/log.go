package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/meetmonth/dance-elite/internal/store"
)

// adminLog 在既有事务内追加一条 admin_logs；detail 序列化为 JSON。
// T5 起的后台敏感操作统一经由此助手落审计（action/target_type/target_id/detail）。
func adminLog(ctx context.Context, stores *store.Store, tx *sql.Tx, adminID *int64, action string, targetType *string, targetID *int64, detail map[string]any) error {
	var raw []byte
	if detail != nil {
		var err error
		raw, err = json.Marshal(detail)
		if err != nil {
			return fmt.Errorf("marshal admin log detail: %w", err)
		}
	}
	return stores.Admins.InsertLog(ctx, tx, store.AdminLogParams{
		AdminID:    adminID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     raw,
	})
}
