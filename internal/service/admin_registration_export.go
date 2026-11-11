package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
)

// 导出名单审计动作（admin_logs.action）。
const adminActionRegistrationExport = "admin.registration.export"

// ExportFile 是一次导出结果（文件名 + Excel 内容 + 行数，行数用于审计）。
type ExportFile struct {
	Name    string
	Content []byte
	Rows    int
}

// Export 按赛事导出有效报名名单（API 文档 12.5）：
// 仅 admin / super_admin（路由层控制），导出记 admin_logs（导出人/赛事/行数）。
// 导出列固定且不包含证件号任何形态（系统只存 HMAC 哈希）。
func (s *AdminRegistrationQueryService) Export(ctx context.Context, adminID, eventID int64, includeCanceled bool) (*ExportFile, error) {
	if _, err := s.stores.Events.Get(ctx, eventID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.ErrNotFound("赛事不存在")
		}
		return nil, httpx.ErrInternal(err, "加载赛事失败，请稍后重试")
	}

	rows, err := s.stores.Registrations.ListForExport(ctx, eventID, includeCanceled)
	if err != nil {
		return nil, httpx.ErrInternal(err, "加载报名名单失败，请稍后重试")
	}
	content, err := buildRegistrationWorkbook(rows)
	if err != nil {
		return nil, httpx.ErrInternal(err, "生成导出文件失败，请稍后重试")
	}

	err = s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		return adminLog(ctx, s.stores, tx, &adminID, adminActionRegistrationExport,
			strPtr("event"), &eventID, map[string]any{
				"rows":             len(rows),
				"include_canceled": includeCanceled,
			})
	})
	if err != nil {
		return nil, httpx.ErrInternal(err, "记录操作日志失败，请稍后重试")
	}

	name := fmt.Sprintf("registrations_%d_%s.xlsx", eventID, s.now().Format("20060102"))
	return &ExportFile{Name: name, Content: content, Rows: len(rows)}, nil
}
