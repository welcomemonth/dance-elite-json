// Package logger wraps zap with a process-wide logger plus context-scoped
// request logging.
//
// Layering rule: only wiring code (main / router / middleware) logs, via
// the global Log or a context logger from FromCtx. Business code
// (service / model) stays log-free — it returns errors and the handler
// layer records them via httpx.BizError.Cause — keeping services
// unit-testable without log capture.
package logger

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Log is the process-wide logger. Defaults to a no-op logger so packages
// stay safe to use in tests before Init runs; main calls Init once.
var Log = zap.NewNop().Sugar()

// New builds a logger: JSON to stdout in production, colored console in debug.
// Output goes to stdout/stderr only; the supervisor collects and rotates logs.
func New(debug bool) (*zap.SugaredLogger, error) {
	cfg := zap.NewProductionConfig()
	if debug {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	l, err := cfg.Build(
		zap.AddCaller(),                       // file:line of the call site
		zap.AddStacktrace(zapcore.ErrorLevel), // Error and above carry a stacktrace
	)
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	return l.Sugar(), nil
}

// Init builds and installs the global logger. Called once from main
// (composition root), after config is loaded.
func Init(debug bool) error {
	l, err := New(debug)
	if err != nil {
		return err
	}
	Log = l
	return nil
}

type ctxKey struct{}

// Ctx returns a context carrying l, for propagating a request-scoped
// logger (e.g. with request_id attached) downstream.
func Ctx(ctx context.Context, l *zap.SugaredLogger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromCtx returns the logger stored in ctx, falling back to the global Log.
// Wiring code should log with FromCtx so request-scoped fields flow through.
func FromCtx(ctx context.Context) *zap.SugaredLogger {
	if l, ok := ctx.Value(ctxKey{}).(*zap.SugaredLogger); ok && l != nil {
		return l
	}
	return Log
}
