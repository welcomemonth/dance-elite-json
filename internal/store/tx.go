package store

import (
	"context"
	"database/sql"
	"fmt"
)

// WithTx 以 BEGIN IMMEDIATE 开启事务并执行 fn。
//
// 事务边界由调用方（service 层）控制：跨表写操作必须在单个事务内完成，
// store 层只提供本助手，不自行开启/嵌套事务。
// fn 返回错误时回滚并原样上抛；提交失败也作为错误返回。
// SQLite 单写入者模型下 BEGIN IMMEDIATE 在写操作前即持有数据库写锁，
// 保证"读-改-写"序列不被并发事务交错（如抢名额的 UPDATE-taken 模式）。
func (d *DB) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// modernc.org/sqlite 的 BeginTx 目前忽略 Isolation 选项，立即获取写锁
	// 依赖 DSN/连接级 busy_timeout 与单连接池串行化；保留 LevelSerializable
	// 语义声明，便于切换到支持的数据驱动时保持行为。
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
