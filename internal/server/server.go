package server

import (
	"context"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/pkg/logger"
	"github.com/meetmonth/dance-elite/internal/router"
	"github.com/meetmonth/dance-elite/internal/service"
	"github.com/meetmonth/dance-elite/internal/store"
)

type Server struct {
	port   string
	engine *gin.Engine
}

func NewServer(port, dsn string) *Server {
	engine := gin.Default()
	store := store.NewStore(dsn)
	apiService := service.NewService(store)
	router.Setup(engine, apiService)

	return &Server{
		port:   port,
		engine: engine,
	}
}

func (s *Server) Run() error {
	srv := &http.Server{
		Addr:    ":" + s.port,
		Handler: s.engine,
	}

	// 创建可取消的上下文监听系统信号
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 启动服务器
	go func() {
		logger.Infof("服务器启动在 %s", s.port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("服务器启动失败: %v", err)
		}
	}()

	// 等待关闭信号
	<-ctx.Done()
	logger.Infof("收到关闭信号，正在优雅关闭...")

	// 给 5 秒时间处理完当前请求
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("服务器关闭失败: %w", err)
	}

	logger.Infof("服务器已关闭")
	return nil
}
