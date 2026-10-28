package logger

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestVictoriaLogsEncoder_Basic(t *testing.T) {
	encoder := &victoriaLogsEncoder{
		streamFields: []string{"level", "component"},
	}

	record := slog.NewRecord(time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.String("user_id", "123"), slog.Int("request_id", 456))

	data, err := encoder.encodeRecords([]slog.Record{record})
	if err != nil {
		t.Fatalf("encodeRecords failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry["_msg"] != "test message user_id=123 request_id=456" {
		t.Errorf("_msg = %v, want 'test message user_id=123 request_id=456'", entry["_msg"])
	}

	if entry["user_id"] != "123" {
		t.Errorf("user_id = %v, want '123'", entry["user_id"])
	}

	if entry["request_id"] != float64(456) {
		t.Errorf("request_id = %v, want 456.0", entry["request_id"])
	}

	if _, hasStream := entry["_stream"]; !hasStream {
		t.Error("_stream field missing")
	}
}

func TestVictoriaLogsEncoder_ContentType(t *testing.T) {
	encoder := &victoriaLogsEncoder{}
	if ct := encoder.contentType(); ct != "application/stream+json" {
		t.Errorf("contentType = %s, want application/stream+json", ct)
	}
}

func TestVictoriaLogsEncoder_MultipleRecords(t *testing.T) {
	encoder := &victoriaLogsEncoder{
		streamFields: []string{"level"},
	}

	records := []slog.Record{
		slog.NewRecord(time.Now(), slog.LevelDebug, "debug msg", 0),
		slog.NewRecord(time.Now(), slog.LevelInfo, "info msg", 0),
	}

	data, err := encoder.encodeRecords(records)
	if err != nil {
		t.Fatalf("encodeRecords failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Verify each line is valid JSON
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}
}
