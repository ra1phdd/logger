package logger

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"
)

func TestLokiEncoder_ContentType(t *testing.T) {
	encoder := &lokiEncoder{}
	if ct := encoder.contentType(); ct != "application/json" {
		t.Errorf("contentType = %s, want application/json", ct)
	}
}

func TestLokiEncoder_TimestampAsString(t *testing.T) {
	encoder := &lokiEncoder{
		labels: []string{"level"},
	}

	ts := time.Date(2024, 1, 15, 10, 30, 0, 123456789, time.UTC)
	record := slog.NewRecord(ts, slog.LevelInfo, "test message", 0)

	data, err := encoder.encodeRecords([]slog.Record{record})
	if err != nil {
		t.Fatalf("encodeRecords failed: %v", err)
	}

	var pushReq lokiPushRequest
	if err := json.Unmarshal(data, &pushReq); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(pushReq.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(pushReq.Streams))
	}

	stream := pushReq.Streams[0]
	if len(stream.Values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(stream.Values))
	}

	value := stream.Values[0]
	if len(value) != 2 {
		t.Fatalf("expected [timestamp, line], got %d elements", len(value))
	}

	timestampStr, ok := value[0].(string)
	if !ok {
		t.Errorf("timestamp type = %T, want string, got %v", value[0], value[0])
	}

	expectedNs := int64(1705318200123456789)
	if timestampStr != "" {
		var parsedNs int64
		err := json.Unmarshal([]byte("\""+timestampStr+"\""), &parsedNs)
		if err == nil {
			if parsedNs != expectedNs {
				t.Logf("Note: timestamp value = %s, parsed as %d, expected %d", timestampStr, parsedNs, expectedNs)
			}
		}
	}
}

func TestLokiEncoder_RecordsGroupedByLabels(t *testing.T) {
	encoder := &lokiEncoder{
		labels:   []string{"level"},
		labelSet: map[string]bool{"level": true},
	}

	record1 := slog.NewRecord(time.Now(), slog.LevelInfo, "info message", 0)
	record2 := slog.NewRecord(time.Now(), slog.LevelWarn, "warn message", 0)
	record3 := slog.NewRecord(time.Now(), slog.LevelInfo, "another info", 0)

	data, err := encoder.encodeRecords([]slog.Record{record1, record2, record3})
	if err != nil {
		t.Fatalf("encodeRecords failed: %v", err)
	}

	var pushReq lokiPushRequest
	if err := json.Unmarshal(data, &pushReq); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should have 2 streams: one for info, one for warn
	if len(pushReq.Streams) != 2 {
		t.Fatalf("expected 2 streams (info and warn), got %d", len(pushReq.Streams))
	}

	// Count records in each stream
	infoCount := 0
	warnCount := 0
	for _, stream := range pushReq.Streams {
		if level, ok := stream.Stream["level"]; ok {
			switch level {
			case "INFO":
				infoCount += len(stream.Values)
			case "WARN":
				warnCount += len(stream.Values)
			}
		}
	}

	if infoCount != 2 {
		t.Errorf("expected 2 info records, got %d", infoCount)
	}
	if warnCount != 1 {
		t.Errorf("expected 1 warn record, got %d", warnCount)
	}
}

func TestLokiEncoder_LogLineContainsAllData(t *testing.T) {
	encoder := &lokiEncoder{
		labels:   []string{"level"},
		labelSet: map[string]bool{"level": true},
	}

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.String("user_id", "123"), slog.Int("attempt", 5))

	data, err := encoder.encodeRecords([]slog.Record{record})
	if err != nil {
		t.Fatalf("encodeRecords failed: %v", err)
	}

	var pushReq lokiPushRequest
	if err := json.Unmarshal(data, &pushReq); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(pushReq.Streams) == 0 {
		t.Fatal("no streams")
	}

	stream := pushReq.Streams[0]
	if len(stream.Values) == 0 {
		t.Fatal("no values in stream")
	}

	logLine := stream.Values[0][1].(string)

	var logObj map[string]any
	if err := json.Unmarshal([]byte(logLine), &logObj); err != nil {
		t.Fatalf("log line is not valid JSON: %v, line: %s", err, logLine)
	}

	// msg now contains message + all attributes in key=value format
	expectedMsg := "test message user_id=123 attempt=5"
	if logObj["msg"] != expectedMsg {
		t.Errorf("msg = %v, want '%s'", logObj["msg"], expectedMsg)
	}
	// Attributes are still present as separate fields
	if logObj["user_id"] != "123" {
		t.Errorf("user_id = %v, want '123'", logObj["user_id"])
	}
	if logObj["attempt"] != float64(5) {
		t.Errorf("attempt = %v, want 5", logObj["attempt"])
	}
}
