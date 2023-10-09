package logging

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerKeyType int

const loggerKey loggerKeyType = iota

var logger *zap.SugaredLogger

func init() {
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	config.TimeKey = "timestamp"
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout)),
		zap.InfoLevel,
	)
	logger = zap.New(core, zap.Fields()).Sugar()
}

// NewContext returns a new context with the logger
func NewContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerKey, WithContext(ctx))
}

// WithContext returns a sugared logger with the context
func WithContext(ctx context.Context) *zap.SugaredLogger {
	if ctx == nil {
		return logger
	}
	if l, ok := ctx.Value(loggerKey).(*zap.SugaredLogger); ok {
		return l
	}
	return logger
}
