package main

import (
	"fmt"
	"os"

	"github.com/meetmonth/dance-elite/internal/pkg/logger"
	"github.com/meetmonth/dance-elite/internal/server"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func init() {
	err := logger.Init(false)
	if err != nil {
		fmt.Println("logger init failed")
		os.Exit(-1)
	}
}

func main() {
	// 1. 定义命令行参数及默认值
	// pflag 原生支持 --port 和 --dsn 这样的长格式参数
	pflag.String("port", "8080", "服务器监听端口")
	pflag.String("dsn", "./data", "数据源路径 (Data Source Name)")

	// 2. 解析命令行参数
	pflag.Parse()

	// 3. 将 pflag 绑定到 viper
	// 这样 viper 就可以统一管理配置（后续如果需要加入配置文件或环境变量，也会非常方便）
	_ = viper.BindPFlag("port", pflag.Lookup("port"))
	_ = viper.BindPFlag("dsn", pflag.Lookup("dsn"))

	// 4. 从 viper 读取参数
	port := viper.GetString("port")
	dsn := viper.GetString("dsn")

	// 启动服务 (注意：变量名改为 srv，避免与包名 server 冲突)
	srv := server.NewServer(port, dsn)
	if err := srv.Run(); err != nil {
		logger.Fatalf("服务器运行失败: %v", err)
	}
}
