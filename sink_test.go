package logger

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

// mockSink captures records for verification.
type mockSink struct {
	mu      sync.Mutex
	records []slog.Record
	enabled []struct {
		ctx   context.Context
		level slog.Level
	}
}

func (m *mockSink) Enabled(ctx context.Context, level slog.Level) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = append(m.enabled, struct {
		ctx   context.Context
		level slog.Level
	}{ctx, level})
	return true
}

func (m *mockSink) Handle(ctx context.Context, r slog.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, r.Clone())
	return nil
}

func (m *mockSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	return m
}

func (m *mockSink) WithGroup(name string) slog.Handler {
	return m
}

func TestLevelFilterHandler_FiltersCorrectly(t *testing.T) {
	mock := &mockSink{}
	globalLevel := &slog.LevelVar{}
	globalLevel.Set(slog.LevelDebug) // global floor is Debug

	sinkLevel := &slog.LevelVar{}
	sinkLevel.Set(slog.LevelWarn) // sink floor is Warn

	handler := &levelFilterHandler{
		handler:     mock,
		globalLevel: globalLevel,
		sinkLevel:   sinkLevel,
	}

	ctx := context.Background()

	// Debug: blocked by sink floor
	if handler.Enabled(ctx, slog.LevelDebug) {
		t.Error("Debug should be blocked when sink level is Warn")
	}

	// Info: blocked by sink floor
	if handler.Enabled(ctx, slog.LevelInfo) {
		t.Error("Info should be blocked when sink level is Warn")
	}

	// Warn: passes
	if !handler.Enabled(ctx, slog.LevelWarn) {
		t.Error("Warn should pass when sink level is Warn")
	}

	// Error: passes
	if !handler.Enabled(ctx, slog.LevelError) {
		t.Error("Error should pass when sink level is Warn")
	}
}

func TestLevelFilterHandler_GlobalFloorEnforced(t *testing.T) {
	mock := &mockSink{}
	globalLevel := &slog.LevelVar{}
	globalLevel.Set(slog.LevelWarn) // global floor is Warn

	// sink has no floor (nil)
	handler := &levelFilterHandler{
		handler:     mock,
		globalLevel: globalLevel,
		sinkLevel:   nil,
	}

	ctx := context.Background()

	// Debug: blocked by global
	if handler.Enabled(ctx, slog.LevelDebug) {
		t.Error("Debug should be blocked by global Warn floor")
	}

	// Info: blocked by global
	if handler.Enabled(ctx, slog.LevelInfo) {
		t.Error("Info should be blocked by global Warn floor")
	}

	// Warn: passes
	if !handler.Enabled(ctx, slog.LevelWarn) {
		t.Error("Warn should pass")
	}
}

func TestLevelFilterHandler_SinkFloorHigherThanGlobal(t *testing.T) {
	mock := &mockSink{}
	globalLevel := &slog.LevelVar{}
	globalLevel.Set(slog.LevelDebug)

	sinkLevel := &slog.LevelVar{}
	sinkLevel.Set(slog.LevelError) // sink floor higher than global

	handler := &levelFilterHandler{
		handler:     mock,
		globalLevel: globalLevel,
		sinkLevel:   sinkLevel,
	}

	ctx := context.Background()

	// Warn: blocked by sink floor
	if handler.Enabled(ctx, slog.LevelWarn) {
		t.Error("Warn should be blocked when sink floor is Error")
	}

	// Error: passes
	if !handler.Enabled(ctx, slog.LevelError) {
		t.Error("Error should pass")
	}
}
