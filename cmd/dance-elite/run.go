package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	config "github.com/meetmonth/dance-elite/internal/configs"
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
	"github.com/meetmonth/dance-elite/internal/router"
	"github.com/meetmonth/dance-elite/internal/server"
	"github.com/meetmonth/dance-elite/internal/store"
)

// run 是组合根：在这里装配数据库、种子、路由与 HTTP 服务，并随 ctx 优雅退出。
// 返回错误由 main 统一记录，业务装配过程不 panic。
func run(ctx context.Context, cfg *config.Config) error {
	if dir := filepath.Dir(cfg.DBPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建数据库目录: %w", err)
		}
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("打开数据库: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Log.Errorf("关闭数据库失败: %v", err)
		}
	}()

	if err := store.Migrate(ctx, db.SQL()); err != nil {
		return fmt.Errorf("数据库迁移: %w", err)
	}
	if err := db.Seed(ctx, store.SeedOptions{
		AdminUsername: cfg.AdminInitUsername,
		AdminPassword: cfg.AdminInitPassword,
	}); err != nil {
		return fmt.Errorf("写入种子数据: %w", err)
	}

	engine := router.New(router.Deps{Stores: store.New(db)})
	srv := server.NewServer(":"+cfg.Port, engine)
	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("HTTP 服务: %w", err)
	}
	return nil
}
