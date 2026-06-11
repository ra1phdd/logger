package logger

import (
	"context"
	"sync"
)

var (
	defaultMu     sync.RWMutex
	defaultLogger *Logger
)

func Init(opts ...Option) {
	SetDefault(New(opts...))
}

func Default() *Logger {
	defaultMu.RLock()
	l := defaultLogger
	defaultMu.RUnlock()
	if l != nil {
		return l
	}

	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultLogger == nil {
		defaultLogger = New()
	}
	return defaultLogger
}

func SetDefault(l *Logger) {
	defaultMu.Lock()
	defer defaultMu.Unlock()

	defaultLogger = l
}

func SetLogLevel(level string) {
	Default().SetLogLevel(level)
}

func GetLogLevel() string {
	return Default().GetLogLevel()
}

func Named(component string) *Logger {
	return Default().Named(component)
}

func With(attrs ...Attr) *Logger {
	return Default().With(attrs...)
}

func Trace(msg string, attrs ...Attr) {
	Default().Trace(msg, attrs...)
}

func Debug(msg string, attrs ...Attr) {
	Default().Debug(msg, attrs...)
}

func Info(msg string, attrs ...Attr) {
	Default().Info(msg, attrs...)
}

func Warn(msg string, attrs ...Attr) {
	Default().Warn(msg, attrs...)
}

func Error(msg string, attrs ...Attr) {
	Default().Error(msg, attrs...)
}

func Fatal(msg string, attrs ...Attr) {
	Default().Fatal(msg, attrs...)
}

func TraceContext(ctx context.Context, msg string, attrs ...Attr) {
	FromContext(ctx).TraceContext(ctx, msg, attrs...)
}

func DebugContext(ctx context.Context, msg string, attrs ...Attr) {
	FromContext(ctx).DebugContext(ctx, msg, attrs...)
}

func InfoContext(ctx context.Context, msg string, attrs ...Attr) {
	FromContext(ctx).InfoContext(ctx, msg, attrs...)
}

func WarnContext(ctx context.Context, msg string, attrs ...Attr) {
	FromContext(ctx).WarnContext(ctx, msg, attrs...)
}

func ErrorContext(ctx context.Context, msg string, attrs ...Attr) {
	FromContext(ctx).ErrorContext(ctx, msg, attrs...)
}

func FatalContext(ctx context.Context, msg string, attrs ...Attr) {
	FromContext(ctx).FatalContext(ctx, msg, attrs...)
}
