package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.SugaredLogger

// 日志系统初始化，代码中的日志文件只写入stdout/stderr，
// 由supervisor管理程序负责日志的收集和写入
// 推荐新版本使用New而非Init的全局变量，方便进行测试
func New(debug bool) (*zap.SugaredLogger, error) {
	var config *zap.Config
	if !debug {
		config = &zap.Config{
			Level:       zap.NewAtomicLevelAt(zap.InfoLevel),
			Development: false,
			Encoding:    "json", // 生成用JSON
			EncoderConfig: zapcore.EncoderConfig{
				TimeKey:       "time",
				LevelKey:      "level",
				MessageKey:    "msg",
				CallerKey:     "caller",
				StacktraceKey: "stacktrace",
				EncodeCaller:  zapcore.ShortCallerEncoder,
				EncodeTime:    zapcore.ISO8601TimeEncoder,
				EncodeLevel:   zapcore.LowercaseLevelEncoder,
			},
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
		}
	} else {
		config = &zap.Config{
			Level:       zap.NewAtomicLevelAt(zap.DebugLevel),
			Development: true,
			Encoding:    "console", // 开发用 console
			EncoderConfig: zapcore.EncoderConfig{
				TimeKey:       "time",
				LevelKey:      "level",
				MessageKey:    "msg",
				CallerKey:     "caller",
				StacktraceKey: "stacktrace",
				EncodeCaller:  zapcore.ShortCallerEncoder,
				EncodeTime:    zapcore.ISO8601TimeEncoder,
				EncodeLevel:   zapcore.CapitalColorLevelEncoder,
			},
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
		}
	}
	logger, err := config.Build(
		zap.AddCaller(),                       // 显示文件:行号
		zap.AddCallerSkip(1),                  // 如果封装了日志函数，跳过一层
		zap.AddStacktrace(zapcore.ErrorLevel), // Error 及以上自动带堆栈
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}
	return logger.Sugar(), nil
}

func Init(debug bool) error {
	var err error
	Log, err = New(debug)
	return err
}

// 便捷方法
func Debug(args ...interface{}) { Log.Debug(args...) }
func Info(args ...interface{})  { Log.Info(args...) }
func Warn(args ...interface{})  { Log.Warn(args...) }
func Error(args ...interface{}) { Log.Error(args...) }
func Fatal(args ...interface{}) { Log.Fatal(args...) }

func Debugf(template string, args ...interface{}) { Log.Debugf(template, args...) }
func Infof(template string, args ...interface{})  { Log.Infof(template, args...) }
func Warnf(template string, args ...interface{})  { Log.Warnf(template, args...) }
func Errorf(template string, args ...interface{}) { Log.Errorf(template, args...) }
func Fatalf(template string, args ...interface{}) { Log.Fatalf(template, args...) }

func Debugw(msg string, args ...interface{}) { Log.Debugw(msg, args...) }
func Infow(msg string, args ...interface{})  { Log.Infow(msg, args...) }
func Warnw(msg string, args ...interface{})  { Log.Warnw(msg, args...) }
func Errorw(msg string, args ...interface{}) { Log.Errorw(msg, args...) }
func Fatalw(msg string, args ...interface{}) { Log.Fatalw(msg, args...) }
