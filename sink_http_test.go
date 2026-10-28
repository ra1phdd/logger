package logger

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type mockEncoder struct {
	encoded []byte
}

func (e *mockEncoder) encodeRecords(records []slog.Record) ([]byte, error) {
	return e.encoded, nil
}

func (e *mockEncoder) contentType() string {
	return "application/json"
}

func TestHTTPSink_RetryOn500(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)
		if count < 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := &DeliveryConfig{
		BufferSize:       10,
		BatchSize:        1,
		FlushInterval:    100 * time.Millisecond,
		MaxRetries:       3,
		RetryInitialWait: 10 * time.Millisecond,
		RetryMaxWait:     50 * time.Millisecond,
	}
	cfg.setDefaults()

	encoder := &mockEncoder{encoded: []byte("test data")}
	level := &slog.LevelVar{}
	level.Set(slog.LevelInfo)
	sinkGrp := newSinkGroup()

	sink := newHTTPSink(server.URL, cfg, encoder, level, sinkGrp)

	sink.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0))

	// Wait for retries to complete
	time.Sleep(500 * time.Millisecond)

	if got := attempts.Load(); got < 2 {
		t.Errorf("expected at least 2 attempts (retry on 500), got %d", got)
	}
}

func TestHTTPSink_NoRetryOn400(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	cfg := &DeliveryConfig{
		BufferSize:       10,
		BatchSize:        1,
		FlushInterval:    100 * time.Millisecond,
		MaxRetries:       3,
		RetryInitialWait: 10 * time.Millisecond,
	}
	cfg.setDefaults()

	encoder := &mockEncoder{encoded: []byte("bad data")}
	level := &slog.LevelVar{}
	level.Set(slog.LevelInfo)
	sinkGrp := newSinkGroup()

	sink := newHTTPSink(server.URL, cfg, encoder, level, sinkGrp)

	sink.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0))

	// Wait a bit
	time.Sleep(300 * time.Millisecond)

	if got := attempts.Load(); got != 1 {
		t.Errorf("expected 1 attempt (no retry on 400), got %d", got)
	}
}

func TestHTTPSink_BufferDropsOnOverflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &DeliveryConfig{
		BufferSize:    2,
		BatchSize:     10, // Never flush by batch size
		FlushInterval: 10 * time.Second, // Way in the future
		MaxRetries:    1,
	}
	cfg.setDefaults()

	encoder := &mockEncoder{encoded: []byte("data")}
	level := &slog.LevelVar{}
	level.Set(slog.LevelInfo)
	sinkGrp := newSinkGroup()

	sink := newHTTPSink(server.URL, cfg, encoder, level, sinkGrp)
	stats := sink.stats

	// Fill buffer
	sink.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "msg1", 0))
	sink.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "msg2", 0))

	// This should drop
	sink.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "msg3", 0))

	// Give it a moment
	time.Sleep(50 * time.Millisecond)

	if stats.droppedOverflow.Load() == 0 {
		t.Error("expected dropped records on overflow, got 0")
	}
}

func TestLevelFilterHandler_SkipsDisabledLevels(t *testing.T) {
	globalLevel := &slog.LevelVar{}
	globalLevel.Set(slog.LevelInfo)

	sinkLevel := &slog.LevelVar{}
	sinkLevel.Set(slog.LevelWarn)

	mockHandler := &mockSink{}
	filterHandler := &levelFilterHandler{
		handler:     mockHandler,
		globalLevel: globalLevel,
		sinkLevel:   sinkLevel,
	}

	// Debug should be skipped (below sink level)
	if filterHandler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Debug should be disabled when sink level is Warn")
	}

	// Warn should pass
	if !filterHandler.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("Warn should be enabled when sink level is Warn")
	}
}
