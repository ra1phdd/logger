package logger

import (
	"bytes"
	"log/slog"
	"testing"
)

func BenchmarkLoggerInfo(b *testing.B) {
	buf := &bytes.Buffer{}
	l := New(
		WithOutput(buf),
		WithNoColor(true),
		WithLevel(slog.LevelInfo),
	)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Info("benchmark message",
				slog.String("key", "value"),
				slog.Int("count", 42),
			)
		}
	})
	_ = l.Shutdown()
}

func BenchmarkLoggerWithAttrs(b *testing.B) {
	buf := &bytes.Buffer{}
	l := New(
		WithOutput(buf),
		WithNoColor(true),
		WithLevel(slog.LevelInfo),
	)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.With(
				slog.String("component", "test"),
				slog.String("session", "abc123"),
			).Info("message")
		}
	})
	_ = l.Shutdown()
}

func BenchmarkLoggerNamed(b *testing.B) {
	buf := &bytes.Buffer{}
	l := New(
		WithOutput(buf),
		WithNoColor(true),
		WithLevel(slog.LevelInfo),
	)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Named("component").Info("message",
				slog.String("key", "value"),
			)
		}
	})
	_ = l.Shutdown()
}
