package logger

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestConsoleLevelFiltering(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	l := New(
		WithOutput(buf),
		WithoutFile(),
		WithNoColor(true),
		WithLevel(slog.LevelDebug),
		WithConsoleLevelString("warn"),
	)

	l.Debug("debug message")
	l.Info("info message")
	l.Warn("warn message")
	l.Error("error message")

	// Wait for async writes to complete
	if err := l.Shutdown(); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	output := buf.String()

	// Debug, Info should not appear (console floor is Warn)
	if len(output) == 0 {
		t.Fatal("Expected output, got empty")
	}
	if bytes.Contains([]byte(output), []byte("debug message")) {
		t.Error("debug message should be filtered out")
	}
	if bytes.Contains([]byte(output), []byte("info message")) {
		t.Error("info message should be filtered out")
	}
	if !bytes.Contains([]byte(output), []byte("warn message")) {
		t.Error("warn message should appear")
	}
	if !bytes.Contains([]byte(output), []byte("error message")) {
		t.Error("error message should appear")
	}
}

func TestWithoutConsole(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	l := New(
		WithOutput(buf),
		WithoutConsole(),
		WithoutFile(),
	)

	l.Info("test message")

	if buf.Len() > 0 {
		t.Errorf("Expected no console output with WithoutConsole(), got %s", buf.String())
	}
}
