package logger

import (
	"io"
	"testing"

	"gopkg.in/natefinch/lumberjack.v2"
)

func TestTypedNilLumberjackLogger(t *testing.T) {
	// Симулируем ситуацию когда newRollingLogFile возвращает typed nil
	var logger *lumberjack.Logger = nil

	// Преобразуем в io.Writer
	var writer io.Writer = logger

	// Проверяем что writer != nil (typed nil)
	//nolint:staticcheck // SA4023: this is intentional to demonstrate typed nil problem
	if writer == nil {
		t.Log("writer is nil")
	} else {
		t.Log("writer is NOT nil (typed nil)")

		// Попытка записи вызовет панику
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Panic as expected: %v", r)
			}
		}()

		// Это вызовет панику
		_, _ = writer.Write([]byte("test"))
		t.Fatal("Expected panic but didn't get one")
	}
}

func TestNormalizeWriterWithTypedNil(t *testing.T) {
	// Создаём typed nil
	var logger *lumberjack.Logger = nil
	var writer io.Writer = logger

	// normalizeWriter должен обнаружить typed nil
	result := normalizeWriter(writer)

	if result != nil {
		t.Errorf("normalizeWriter should return nil for typed nil, got: %v", result)
	}
}
