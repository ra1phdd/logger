package logger

import (
	"log/slog"
	"strings"
)

func parseLevel(level string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace":
		return levelTrace, true
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	case "fatal":
		return levelFatal, true
	default:
		return slog.LevelInfo, false
	}
}

func levelString(level slog.Level) string {
	switch {
	case level <= levelTrace:
		return "trace"
	case level < slog.LevelInfo:
		return "debug"
	case level < slog.LevelWarn:
		return "info"
	case level < slog.LevelError:
		return "warn"
	case level < levelFatal:
		return "error"
	default:
		return "fatal"
	}
}

func levelLabel(level slog.Level) string {
	switch {
	case level <= levelTrace:
		return "TRACE"
	case level < slog.LevelInfo:
		return "DEBUG"
	case level < slog.LevelWarn:
		return "INFO"
	case level < slog.LevelError:
		return "WARN"
	case level < levelFatal:
		return "ERROR"
	default:
		return "FATAL"
	}
}

func levelColor(level slog.Level) uint8 {
	switch {
	case level <= levelTrace:
		return 13
	case level < slog.LevelInfo:
		return 14
	case level < slog.LevelWarn:
		return 10
	case level < slog.LevelError:
		return 11
	case level < levelFatal:
		return 9
	default:
		return 13
	}
}
