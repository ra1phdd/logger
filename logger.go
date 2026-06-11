package logger

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

var backgroundContext = context.Background()

type Logger struct {
	*slog.Logger
	handler   slog.Handler
	component string
	level     *slog.LevelVar
	exit      func(int)
}

type SlogLogger = Logger

func New(opts ...Option) *Logger {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	level := &slog.LevelVar{}
	level.Set(cfg.level)

	handler := newRootHandler(cfg, level)
	return &Logger{
		Logger:    slog.New(handler),
		handler:   handler,
		component: cfg.component,
		level:     level,
		exit:      cfg.exit,
	}
}

func (l *Logger) With(attrs ...Attr) *Logger {
	if l == nil {
		return Default().With(attrs...)
	}
	return l.clone(l.handler.WithAttrs(attrs))
}

func (l *Logger) WithGroup(name string) *Logger {
	if l == nil {
		return Default().WithGroup(name)
	}
	return l.clone(l.handler.WithGroup(name))
}

func (l *Logger) Named(component string) *Logger {
	if l == nil {
		return Default().Named(component)
	}
	component = strings.TrimSpace(component)
	if component == "" {
		return l
	}
	handler := l.handler
	if withComponentHandler, ok := handler.(interface{ WithComponent(string) slog.Handler }); ok {
		handler = withComponentHandler.WithComponent(component)
	}
	return l.cloneWithComponent(handler, component)
}

func (l *Logger) Slog() *slog.Logger {
	if l == nil {
		return slog.Default()
	}
	return l.Logger
}

func (l *Logger) Handler() slog.Handler {
	if l == nil || l.handler == nil {
		return slog.Default().Handler()
	}
	return l.handler
}

func (l *Logger) Enabled(ctx context.Context, level slog.Level) bool {
	if l == nil || l.Logger == nil {
		return false
	}
	return l.Logger.Enabled(ctx, level)
}

func (l *Logger) SetLevel(level slog.Level) {
	if l != nil && l.level != nil {
		l.level.Set(level)
	}
}

func (l *Logger) Level() slog.Level {
	if l == nil || l.level == nil {
		return slog.LevelInfo
	}
	return l.level.Level()
}

func (l *Logger) SetLogLevel(level string) {
	if parsed, ok := parseLevel(level); ok {
		l.SetLevel(parsed)
		return
	}
	l.SetLevel(slog.LevelInfo)
}

func (l *Logger) GetLogLevel() string {
	return levelString(l.Level())
}

func (l *Logger) Log(ctx context.Context, level slog.Level, msg string, attrs ...Attr) {
	if l == nil || l.Logger == nil {
		return
	}
	if ctx == nil {
		ctx = backgroundContext
	}
	if !l.Logger.Enabled(ctx, level) {
		return
	}

	record := slog.NewRecord(time.Now(), level, msg, callerPC())
	record.AddAttrs(attrs...)

	_ = l.Logger.Handler().Handle(ctx, record)
}

func (l *Logger) Trace(msg string, attrs ...Attr) {
	l.logWithLevel(backgroundContext, levelTrace, msg, attrs...)
}

func (l *Logger) Debug(msg string, attrs ...Attr) {
	l.logWithLevel(backgroundContext, slog.LevelDebug, msg, attrs...)
}

func (l *Logger) Info(msg string, attrs ...Attr) {
	l.logWithLevel(backgroundContext, slog.LevelInfo, msg, attrs...)
}

func (l *Logger) Warn(msg string, attrs ...Attr) {
	l.logWithLevel(backgroundContext, slog.LevelWarn, msg, attrs...)
}

func (l *Logger) Error(msg string, attrs ...Attr) {
	l.logWithLevel(backgroundContext, slog.LevelError, msg, attrs...)
}

func (l *Logger) Fatal(msg string, attrs ...Attr) {
	l.logWithLevel(backgroundContext, levelFatal, msg, attrs...)
	l.exitIfNeeded()
}

func (l *Logger) TraceContext(ctx context.Context, msg string, attrs ...Attr) {
	l.logWithLevel(ctx, levelTrace, msg, attrs...)
}

func (l *Logger) DebugContext(ctx context.Context, msg string, attrs ...Attr) {
	l.logWithLevel(ctx, slog.LevelDebug, msg, attrs...)
}

func (l *Logger) InfoContext(ctx context.Context, msg string, attrs ...Attr) {
	l.logWithLevel(ctx, slog.LevelInfo, msg, attrs...)
}

func (l *Logger) WarnContext(ctx context.Context, msg string, attrs ...Attr) {
	l.logWithLevel(ctx, slog.LevelWarn, msg, attrs...)
}

func (l *Logger) ErrorContext(ctx context.Context, msg string, attrs ...Attr) {
	l.logWithLevel(ctx, slog.LevelError, msg, attrs...)
}

func (l *Logger) FatalContext(ctx context.Context, msg string, attrs ...Attr) {
	l.logWithLevel(ctx, levelFatal, msg, attrs...)
	l.exitIfNeeded()
}

func (l *Logger) logWithLevel(ctx context.Context, level slog.Level, msg string, attrs ...Attr) {
	l.Log(ctx, level, msg, attrs...)
}

func (l *Logger) exitIfNeeded() {
	if l != nil && l.exit != nil {
		l.exit(1)
	}
}

func (l *Logger) clone(handler slog.Handler) *Logger {
	if l == nil {
		return Default().clone(handler)
	}
	return &Logger{
		Logger:    slog.New(handler),
		handler:   handler,
		component: l.component,
		level:     l.level,
		exit:      l.exit,
	}
}

func (l *Logger) cloneWithComponent(handler slog.Handler, component string) *Logger {
	if l == nil {
		return Default().cloneWithComponent(handler, component)
	}
	return &Logger{
		Logger:    slog.New(handler),
		handler:   handler,
		component: component,
		level:     l.level,
		exit:      l.exit,
	}
}
