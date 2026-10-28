package logger

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerWorksWhenFileCreationFails(t *testing.T) {
	// Создаём путь к файлу, который нельзя создать
	invalidPath := filepath.Join(os.TempDir(), "nonexistent-dir-with-file/occupied/test.log")

	// Создаём буфер для консольного вывода
	consoleBuf := &bytes.Buffer{}

	// Создаём логгер с невалидным путём к файлу
	logger := New(
		WithFile(invalidPath),
		WithOutput(consoleBuf),
		WithNoColor(true),
	)

	// Пытаемся залогировать - не должно быть паники
	logger.Info("test message")
	logger.Error("test error")
	logger.Named("component").Warn("test warning")

	// Wait for async writes
	_ = logger.Shutdown()

	// Проверяем, что логи попали в консоль
	output := consoleBuf.String()
	if output == "" {
		t.Fatal("Expected console output, got empty string")
	}

	if !strings.Contains(output, "test message") {
		t.Errorf("Expected 'test message' in console output, got: %s", output)
	}

	if !strings.Contains(output, "test error") {
		t.Errorf("Expected 'test error' in console output, got: %s", output)
	}

	if !strings.Contains(output, "test warning") {
		t.Errorf("Expected 'test warning' in console output, got: %s", output)
	}
}

func TestLoggerWithNamedWorksWhenFileCreationFails(t *testing.T) {
	// Создаём путь к файлу, который нельзя создать
	invalidPath := filepath.Join(os.TempDir(), "nonexistent-dir/test.log")

	// Создаём буфер для консольного вывода
	consoleBuf := &bytes.Buffer{}

	// Создаём логгер с невалидным путём к файлу
	logger := New(
		WithFile(invalidPath),
		WithOutput(consoleBuf),
		WithNoColor(true),
	)

	// Создаём named логгер (это вызывает WithComponent)
	namedLogger := logger.Named("mycomponent")

	// Пытаемся залогировать - не должно быть паники
	namedLogger.Info("test from named logger")
	namedLogger.Error("error from named logger")

	// Wait for async writes
	_ = logger.Shutdown()

	// Проверяем, что логи попали в консоль
	output := consoleBuf.String()
	if output == "" {
		t.Fatal("Expected console output, got empty string")
	}

	if !strings.Contains(output, "test from named logger") {
		t.Errorf("Expected 'test from named logger' in console output, got: %s", output)
	}

	if !strings.Contains(output, "error from named logger") {
		t.Errorf("Expected 'error from named logger' in console output, got: %s", output)
	}
}

func TestLoggerWithMultipleLevelsWhenFileCreationFails(t *testing.T) {
	invalidPath := "/this/path/does/not/exist/app.log"
	consoleBuf := &bytes.Buffer{}

	logger := New(
		WithFile(invalidPath),
		WithOutput(consoleBuf),
		WithLevel(slog.LevelDebug),
		WithNoColor(true),
	)

	// Логируем на разных уровнях
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	// Wait for async writes
	_ = logger.Shutdown()

	output := consoleBuf.String()
	if output == "" {
		t.Fatal("Expected console output, got empty string")
	}

	// Проверяем что все сообщения есть
	expectedMessages := []string{"debug message", "info message", "warn message", "error message"}
	for _, msg := range expectedMessages {
		if !strings.Contains(output, msg) {
			t.Errorf("Expected '%s' in output, got: %s", msg, output)
		}
	}
}
