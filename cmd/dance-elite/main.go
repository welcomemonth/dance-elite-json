package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	config "github.com/meetmonth/dance-elite/internal/configs"
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
	"github.com/meetmonth/dance-elite/internal/router"
	"github.com/meetmonth/dance-elite/internal/server"
	"github.com/meetmonth/dance-elite/internal/service"
	"github.com/meetmonth/dance-elite/internal/store"
)

func main() {
	// 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if !cfg.Server.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	// 5. 组合根（composition root）：在这里完成全部依赖装配
	st := store.NewStore(cfg.Database.DSN)
	svc := service.NewService(st)
	engine := router.New(svc)
	srv := server.NewServer(":"+cfg.Server.Port, engine)

	// 6. 监听系统信号，用于触发优雅关闭
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		logger.Fatalf("服务器运行失败: %v", err)
	}
}
