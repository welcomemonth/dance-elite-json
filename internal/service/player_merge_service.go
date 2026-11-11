package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/store"
)

// adminActionPlayerMerge 是档案合并的审计动作（admin_logs.action）。
const adminActionPlayerMerge = "admin.player.merge"

// Merge 合并重复选手档案（开发计划 T16，API 文档 13.2）：
// 把 source 档案的历史报名/成绩/积分/总决赛资格/奖金迁移到 target（保留档案），
// source.merged_into 指向 target。敏感操作，仅管理员，全程 admin_logs 留痕。
//
// 两个档案存在同赛事成绩、同榜单积分等冲突数据时唯一约束会回滚整笔事务，
// 返回 40901 提示人工核对，不做自动去重。
func (s *PlayerService) Merge(ctx context.Context, adminID, sourceID, targetID int64) (*model.MergePlayerResponse, error) {
	if sourceID <= 0 || targetID <= 0 {
		return nil, httpx.ErrBadRequest("选手 ID 不合法")
	}
	if sourceID == targetID {
		return nil, httpx.ErrBadRequest("不能把档案合并到自身")
	}

	var resp *model.MergePlayerResponse
	err := s.stores.WithTx(ctx, func(tx *sql.Tx) error {
		source, err := s.stores.Players.GetByIDTx(ctx, tx, sourceID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("待合并档案不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载选手档案失败，请稍后重试")
		}
		target, err := s.stores.Players.GetByIDTx(ctx, tx, targetID)
		if errors.Is(err, store.ErrNotFound) {
			return httpx.ErrNotFound("保留档案不存在")
		}
		if err != nil {
			return httpx.ErrInternal(err, "加载选手档案失败，请稍后重试")
		}
		if source.MergedInto != nil {
			return httpx.ErrConflict("该档案已合并至其他档案，不能重复合并")
		}
		if target.MergedInto != nil {
			return httpx.ErrConflict("保留档案已合并至其他档案，不能作为合并目标")
		}

		moved, err := s.stores.Players.MoveDataTx(ctx, tx, source.ID, target.ID)
		if err != nil {
			if store.IsUniqueViolation(err) {
				return httpx.ErrConflict("两个档案存在同赛事成绩或同榜单积分等冲突数据，请先人工核对")
			}
			return httpx.ErrInternal(err, "迁移档案历史数据失败，请稍后重试")
		}
		if err := s.stores.Players.SetMergedIntoTx(ctx, tx, source.ID, target.ID); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return httpx.ErrConflict("该档案已合并至其他档案，不能重复合并")
			}
			return httpx.ErrInternal(err, "更新档案合并指向失败，请稍后重试")
		}
		if err := adminLog(ctx, s.stores, tx, &adminID, adminActionPlayerMerge,
			strPtr("player"), &source.ID, map[string]any{
				"source_player_no": source.PlayerNo,
				"target_player_no": target.PlayerNo,
				"target_player_id": target.ID,
				"moved":            moved,
			}); err != nil {
			return httpx.ErrInternal(err, "记录操作日志失败，请稍后重试")
		}
		resp = &model.MergePlayerResponse{
			PlayerID:   source.ID,
			MergedInto: target.ID,
			Moved:      moved,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}
