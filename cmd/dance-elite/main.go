package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gin-gonic/gin"
	config "github.com/meetmonth/dance-elite/internal/configs"
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
	"github.com/meetmonth/dance-elite/internal/router"
	"github.com/meetmonth/dance-elite/internal/server"
	"github.com/meetmonth/dance-elite/internal/store"
)

func main() {
	// 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if err := logger.Init(cfg.Server.Debug); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	if !cfg.Server.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	// 组合根（composition root）：在这里完成全部依赖装配

	// SQLite 数据层：打开连接并执行迁移，幂等可重复启动
	if err := os.MkdirAll(filepath.Dir(cfg.Database.Path), 0o755); err != nil {
		logger.Log.Fatalf("创建数据库目录失败: %v", err)
	}
	db, err := store.Open(cfg.Database.Path)
	if err != nil {
		logger.Log.Fatalf("打开数据库失败: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Log.Errorf("关闭数据库失败: %v", err)
		}
	}()
	if err := store.Migrate(context.Background(), db.SQL()); err != nil {
		logger.Log.Fatalf("数据库迁移失败: %v", err)
	}

	engine := router.New(router.Deps{Stores: store.New(db)})
	srv := server.NewServer(":"+cfg.Server.Port, engine)

	// 监听系统信号，用于触发优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		logger.Log.Fatalf("服务器运行失败: %v", err)
	}
}
