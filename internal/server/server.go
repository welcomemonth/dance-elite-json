package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/meetmonth/dance-elite/internal/pkg/logger"
)

type Server struct {
	addr    string
	handler http.Handler
}

// NewServer 构造服务器。handler 只需实现 http.Handler，
// 与具体 web 框架（gin）解耦，测试时可传入任意 mock handler。
func NewServer(addr string, handler http.Handler) *Server {
	return &Server{
		addr:    addr,
		handler: handler,
	}
}

// Run 启动服务器并阻塞，直到 ctx 被取消后执行优雅关闭。
//
// ctx 由调用方传入（main 中通过 signal.NotifyContext 监听系统信号），
// 因此测试时可用一个可取消的 context 驱动关闭流程，而不依赖真实信号。
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.addr,
		Handler: s.handler,
	}

	// 监听失败时把错误回传，而非直接 os.Exit，便于上层统一处理与测试断言。
	errCh := make(chan error, 1)
	go func() {
		logger.Log.Infof("服务器启动在 %s", s.addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("服务器启动失败: %w", err)
	case <-ctx.Done():
	}

	logger.Log.Infof("收到关闭信号，正在优雅关闭...")

	// 给 5 秒时间处理完当前请求
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("服务器关闭失败: %w", err)
	}

	logger.Log.Infof("服务器已关闭")
	return nil
}
