package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/meetmonth/dance-elite/internal/pkg/logger"
)

func TestMain(m *testing.M) {
	// Run() 内部会调用包级 logger，测试前先初始化，避免 nil 指针。
	if err := logger.Init(false); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestRun_ListenError(t *testing.T) {
	// 先占用一个端口，让后续 ListenAndServe 必然失败
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占用端口失败: %v", err)
	}
	defer ln.Close()

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv := NewServer(ln.Addr().String(), h)

	// 端口被占用，Run 应返回错误而非 os.Exit
	if err := srv.Run(context.Background()); err == nil {
		t.Fatal("端口被占用时 Run 应返回错误")
	}
}
