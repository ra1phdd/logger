package logger

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestHTTPSinkPreservesAttributes verifies that attributes passed via With() and Info()
// reach the encoder and are included in the HTTP payload.
func TestHTTPSinkPreservesAttributes(t *testing.T) {
	var mu sync.Mutex
	var receivedBodies []string

	// Mock server that captures request bodies
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create logger with VictoriaLogs sink
	log := New(
		WithoutConsole(),
		WithoutFile(),
		WithVictoriaLogs(VictoriaLogsConfig{
			URL:          server.URL,
			StreamFields: []string{"level"},
			Delivery: DeliveryConfig{
				BufferSize:    10,
				BatchSize:     2,
				FlushInterval: 100 * time.Millisecond,
			},
		}),
	)

	// Log with attributes
	logWithUser := log.With(String("user_id", "123"))
	logWithUser.Info("test message", String("method", "POST"))
	log.Warn("another message", String("request_id", "abc"))

	// Give time for batch to flush
	time.Sleep(200 * time.Millisecond)
	log.Close(context.Background())

	// Verify received data
	mu.Lock()
	defer mu.Unlock()

	if len(receivedBodies) == 0 {
		t.Fatal("no requests received by mock server")
	}

	// Parse ndjson lines
	allLines := strings.Split(strings.TrimSpace(receivedBodies[0]), "\n")
	if len(allLines) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(allLines))
	}

	// Parse first log line
	var firstLog map[string]any
	if err := json.Unmarshal([]byte(allLines[0]), &firstLog); err != nil {
		t.Fatalf("failed to parse first log line: %v", err)
	}

	// Verify attributes are present
	if firstLog["_msg"] != "test message" {
		t.Errorf("expected msg='test message', got %v", firstLog["_msg"])
	}
	if firstLog["user_id"] != "123" {
		t.Errorf("expected user_id='123', got %v", firstLog["user_id"])
	}
	if firstLog["method"] != "POST" {
		t.Errorf("expected method='POST', got %v", firstLog["method"])
	}
	if firstLog["level"] != "INFO" {
		t.Errorf("expected level='INFO', got %v", firstLog["level"])
	}

	// Parse second log line
	var secondLog map[string]any
	if err := json.Unmarshal([]byte(allLines[1]), &secondLog); err != nil {
		t.Fatalf("failed to parse second log line: %v", err)
	}

	if secondLog["_msg"] != "another message" {
		t.Errorf("expected msg='another message', got %v", secondLog["_msg"])
	}
	if secondLog["request_id"] != "abc" {
		t.Errorf("expected request_id='abc', got %v", secondLog["request_id"])
	}
	if secondLog["level"] != "WARN" {
		t.Errorf("expected level='WARN', got %v", secondLog["level"])
	}
}

// TestHTTPSinkWithGroup verifies that WithGroup creates nested attribute structures.
func TestHTTPSinkWithGroup(t *testing.T) {
	var mu sync.Mutex
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBody = string(body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	log := New(
		WithoutConsole(),
		WithoutFile(),
		WithVictoriaLogs(VictoriaLogsConfig{
			URL: server.URL,
			Delivery: DeliveryConfig{
				BufferSize:    10,
				BatchSize:     1,
				FlushInterval: 100 * time.Millisecond,
			},
		}),
	)

	// Log with group
	log.WithGroup("request").With(slog.String("id", "123")).Info("test", slog.String("method", "GET"))

	time.Sleep(200 * time.Millisecond)
	log.Close(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if receivedBody == "" {
		t.Fatal("no request received")
	}

	lines := strings.Split(strings.TrimSpace(receivedBody), "\n")
	var logLine map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &logLine); err != nil {
		t.Fatalf("failed to parse log: %v", err)
	}

	// Verify nested structure
	// VictoriaLogs flattens groups into dotted keys
	if logLine["request.id"] != "123" {
		t.Errorf("expected request.id='123', got %v", logLine["request.id"])
	}
	if logLine["request.method"] != "GET" {
		t.Errorf("expected request.method='GET', got %v", logLine["request.method"])
	}
}

// TestHTTPSinkNonBlockingOnOverflow verifies that logging doesn't block when buffer is full.
func TestHTTPSinkNonBlockingOnOverflow(t *testing.T) {
	// Slow server that delays responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	log := New(
		WithoutConsole(),
		WithoutFile(),
		WithVictoriaLogs(VictoriaLogsConfig{
			URL: server.URL,
			Delivery: DeliveryConfig{
				BufferSize:    5,  // Small buffer
				BatchSize:     10, // Large batch size, won't trigger
				FlushInterval: 10 * time.Second,
			},
		}),
	)

	// Flood the buffer
	start := time.Now()
	for i := 0; i < 20; i++ {
		log.Info("flood", slog.Int("i", i))
	}
	elapsed := time.Since(start)

	// Should complete instantly (non-blocking)
	if elapsed > 100*time.Millisecond {
		t.Errorf("logging took %v, expected <100ms (blocking detected)", elapsed)
	}

	// Stats should show dropped records
	stats := log.Stats()
	if len(stats) == 0 {
		t.Fatal("no stats returned")
	}

	victorialogsSink := stats[0] // First sink after console/file
	if victorialogsSink.DroppedOverflow == 0 {
		t.Errorf("expected dropped records due to overflow, got 0")
	}

	log.Close(context.Background())
}

// TestHTTPSinkRetryOn5xx verifies retry logic for server errors.
func TestHTTPSinkRetryOn5xx(t *testing.T) {
	var mu sync.Mutex
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		currentAttempt := attempts
		mu.Unlock()

		if currentAttempt < 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	log := New(
		WithoutConsole(),
		WithoutFile(),
		WithVictoriaLogs(VictoriaLogsConfig{
			URL: server.URL,
			Delivery: DeliveryConfig{
				BufferSize:    10,
				BatchSize:     1,
				FlushInterval: 50 * time.Millisecond,
				MaxRetries:    3,
			},
		}),
	)

	log.Info("test retry")
	time.Sleep(300 * time.Millisecond)
	log.Close(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if attempts < 2 {
		t.Errorf("expected at least 2 attempts (retry), got %d", attempts)
	}
}

// TestHTTPSinkNoRetryOn4xx verifies that 4xx responses don't trigger retries.
func TestHTTPSinkNoRetryOn4xx(t *testing.T) {
	var mu sync.Mutex
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		mu.Unlock()
		w.WriteHeader(http.StatusBadRequest) // 400
	}))
	defer server.Close()

	log := New(
		WithoutConsole(),
		WithoutFile(),
		WithVictoriaLogs(VictoriaLogsConfig{
			URL: server.URL,
			Delivery: DeliveryConfig{
				BufferSize:    10,
				BatchSize:     1,
				FlushInterval: 50 * time.Millisecond,
				MaxRetries:    3,
			},
		}),
	)

	log.Info("test no retry")
	time.Sleep(200 * time.Millisecond)
	log.Close(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if attempts != 1 {
		t.Errorf("expected exactly 1 attempt (no retry on 4xx), got %d", attempts)
	}

	// Check that stats reflect the error
	stats := log.Stats()
	if len(stats) > 0 && stats[0].DroppedRetriesExhausted == 0 {
		t.Error("expected DroppedRetriesExhausted > 0 after 4xx")
	}
}
