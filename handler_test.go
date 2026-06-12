package logger

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

func TestRootHandlerEnabled(t *testing.T) {
	t.Run("console handler enables record", func(t *testing.T) {
		// Arrange
		console := newRecordingHandler()
		json := newRecordingHandler()
		console.enabled = true
		json.enabled = false
		handler := rootHandler{console: console, json: json}

		// Act
		enabled := handler.Enabled(context.Background(), slog.LevelInfo)

		// Assert
		if !enabled {
			t.Fatal("Enabled() = false, want true")
		}
	})

	t.Run("json handler enables record when console is disabled", func(t *testing.T) {
		// Arrange
		console := newRecordingHandler()
		json := newRecordingHandler()
		console.enabled = false
		json.enabled = true
		handler := rootHandler{console: console, json: json}

		// Act
		enabled := handler.Enabled(context.Background(), slog.LevelInfo)

		// Assert
		if !enabled {
			t.Fatal("Enabled() = false, want true")
		}
	})

	t.Run("disabled children disable record", func(t *testing.T) {
		// Arrange
		console := newRecordingHandler()
		json := newRecordingHandler()
		console.enabled = false
		json.enabled = false
		handler := rootHandler{console: console, json: json}

		// Act
		enabled := handler.Enabled(context.Background(), slog.LevelInfo)

		// Assert
		if enabled {
			t.Fatal("Enabled() = true, want false")
		}
	})
}

func TestRootHandlerHandleAddsComponentToJSONOnly(t *testing.T) {
	// Arrange
	console := newRecordingHandler()
	json := newRecordingHandler()
	handler := rootHandler{console: console, json: json, component: "api"}
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)

	// Act
	err := handler.Handle(context.Background(), record)

	// Assert
	if err != nil {
		t.Fatalf("Handle() error = %v, want nil", err)
	}
	if len(console.records) != 1 {
		t.Fatalf("console records = %d, want 1", len(console.records))
	}
	if len(json.records) != 1 {
		t.Fatalf("json records = %d, want 1", len(json.records))
	}
	if _, ok := attrByKey(console.records[0], "component"); ok {
		t.Fatal("console record unexpectedly contains component attr")
	}
	attr, ok := attrByKey(json.records[0], "component")
	if !ok {
		t.Fatal("json record is missing component attr")
	}
	if attr.Value.String() != "api" {
		t.Fatalf("component attr = %q, want api", attr.Value.String())
	}
}

func TestRootHandlerHandleJoinsErrors(t *testing.T) {
	// Arrange
	consoleErr := errors.New("console failed")
	jsonErr := errors.New("json failed")
	console := newRecordingHandler()
	json := newRecordingHandler()
	console.handleErr = consoleErr
	json.handleErr = jsonErr
	handler := rootHandler{console: console, json: json}
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)

	// Act
	err := handler.Handle(context.Background(), record)

	// Assert
	if !errors.Is(err, consoleErr) {
		t.Fatalf("Handle() error = %v, want console error", err)
	}
	if !errors.Is(err, jsonErr) {
		t.Fatalf("Handle() error = %v, want json error", err)
	}
}

func TestRootHandlerChildHandlersCarryAttrsAndGroups(t *testing.T) {
	// Arrange
	console := newRecordingHandler()
	json := newRecordingHandler()
	handler := rootHandler{console: console, json: json}

	// Act
	withAttrs, ok := handler.WithAttrs([]slog.Attr{String("requestID", "42")}).(rootHandler)
	if !ok {
		t.Fatal("WithAttrs() did not return rootHandler")
	}
	withGroup, ok := handler.WithGroup("http").(rootHandler)
	if !ok {
		t.Fatal("WithGroup() did not return rootHandler")
	}

	// Assert
	consoleWithAttrs, ok := withAttrs.console.(*recordingHandler)
	if !ok {
		t.Fatalf("console WithAttrs type = %T, want *recordingHandler", withAttrs.console)
	}
	jsonWithAttrs, ok := withAttrs.json.(*recordingHandler)
	if !ok {
		t.Fatalf("json WithAttrs type = %T, want *recordingHandler", withAttrs.json)
	}
	if len(consoleWithAttrs.withAttrs) != 1 || consoleWithAttrs.withAttrs[0].Key != "requestID" {
		t.Fatalf("console withAttrs = %#v, want requestID attr", consoleWithAttrs.withAttrs)
	}
	if len(jsonWithAttrs.withAttrs) != 1 || jsonWithAttrs.withAttrs[0].Key != "requestID" {
		t.Fatalf("json withAttrs = %#v, want requestID attr", jsonWithAttrs.withAttrs)
	}

	consoleWithGroup, ok := withGroup.console.(*recordingHandler)
	if !ok {
		t.Fatalf("console WithGroup type = %T, want *recordingHandler", withGroup.console)
	}
	jsonWithGroup, ok := withGroup.json.(*recordingHandler)
	if !ok {
		t.Fatalf("json WithGroup type = %T, want *recordingHandler", withGroup.json)
	}
	if len(consoleWithGroup.withGroups) != 1 || consoleWithGroup.withGroups[0] != "http" {
		t.Fatalf("console withGroups = %#v, want http", consoleWithGroup.withGroups)
	}
	if len(jsonWithGroup.withGroups) != 1 || jsonWithGroup.withGroups[0] != "http" {
		t.Fatalf("json withGroups = %#v, want http", jsonWithGroup.withGroups)
	}
}

func TestRootHandlerWithComponentUpdatesConsoleHandler(t *testing.T) {
	// Arrange
	console := newRecordingHandler()
	json := newRecordingHandler()
	handler := rootHandler{console: console, json: json}

	// Act
	child, ok := handler.WithComponent("worker").(rootHandler)
	if !ok {
		t.Fatal("WithComponent() did not return rootHandler")
	}

	// Assert
	consoleChild, ok := child.console.(*recordingHandler)
	if !ok {
		t.Fatalf("console child type = %T, want *recordingHandler", child.console)
	}
	if consoleChild.component != "worker" {
		t.Fatalf("console component = %q, want worker", consoleChild.component)
	}
	if child.component != "worker" {
		t.Fatalf("root component = %q, want worker", child.component)
	}
	if child.json != json {
		t.Fatal("json handler unexpectedly changed")
	}
}

func TestConsolePrefixHandlerHandlePrefixesMessage(t *testing.T) {
	// Arrange
	inner := newRecordingHandler()
	record, location := recordForPrefixTest(t, "hello")
	handler := consolePrefixHandler{
		Handler:   inner,
		component: "worker",
		noColor:   true,
	}

	// Act
	err := handler.Handle(context.Background(), record)

	// Assert
	if err != nil {
		t.Fatalf("Handle() error = %v, want nil", err)
	}
	if len(inner.records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(inner.records))
	}
	expected := "worker " + location + " > hello"
	if inner.records[0].Message != expected {
		t.Fatalf("message = %q, want %q", inner.records[0].Message, expected)
	}
}

func TestConsolePrefixHandlerPrefixUsesPathComponentWhenComponentMissing(t *testing.T) {
	// Arrange
	record, location := recordForPrefixTest(t, "hello")
	handler := consolePrefixHandler{noColor: true}

	// Act
	prefix := handler.prefix(record.PC, "")

	// Assert
	expected := "logger " + location + " > "
	if prefix != expected {
		t.Fatalf("prefix = %q, want %q", prefix, expected)
	}
}

func TestConsolePrefixHandlerChildHandlersPreserveMetadata(t *testing.T) {
	// Arrange
	inner := newRecordingHandler()
	handler := consolePrefixHandler{
		Handler:   inner,
		component: "api",
		noColor:   true,
	}

	// Act
	withAttrs, ok := handler.WithAttrs([]slog.Attr{String("requestID", "42")}).(consolePrefixHandler)
	if !ok {
		t.Fatal("WithAttrs() did not return consolePrefixHandler")
	}
	withGroup, ok := handler.WithGroup("http").(consolePrefixHandler)
	if !ok {
		t.Fatal("WithGroup() did not return consolePrefixHandler")
	}
	withComponent, ok := handler.WithComponent("worker").(consolePrefixHandler)
	if !ok {
		t.Fatal("WithComponent() did not return consolePrefixHandler")
	}

	// Assert
	attrsHandler, ok := withAttrs.Handler.(*recordingHandler)
	if !ok {
		t.Fatalf("withAttrs.Handler type = %T, want *recordingHandler", withAttrs.Handler)
	}
	groupHandler, ok := withGroup.Handler.(*recordingHandler)
	if !ok {
		t.Fatalf("withGroup.Handler type = %T, want *recordingHandler", withGroup.Handler)
	}
	if withAttrs.component != "api" || !withAttrs.noColor {
		t.Fatal("WithAttrs() did not preserve console prefix metadata")
	}
	if len(attrsHandler.withAttrs) != 1 || attrsHandler.withAttrs[0].Key != "requestID" {
		t.Fatalf("attrsHandler.withAttrs = %#v, want requestID attr", attrsHandler.withAttrs)
	}
	if withGroup.component != "api" || !withGroup.noColor {
		t.Fatal("WithGroup() did not preserve console prefix metadata")
	}
	if len(groupHandler.withGroups) != 1 || groupHandler.withGroups[0] != "http" {
		t.Fatalf("groupHandler.withGroups = %#v, want http", groupHandler.withGroups)
	}
	if withComponent.component != "worker" {
		t.Fatalf("withComponent.component = %q, want worker", withComponent.component)
	}
}

func TestConsolePrefixHandlerHandleAddsANSIResetWhenColorsEnabled(t *testing.T) {
	// Arrange
	inner := newRecordingHandler()
	record, location := recordForPrefixTest(t, "hello")
	handler := consolePrefixHandler{
		Handler:   inner,
		component: "worker",
		noColor:   false,
	}

	// Act
	err := handler.Handle(context.Background(), record)

	// Assert
	if err != nil {
		t.Fatalf("Handle() error = %v, want nil", err)
	}
	if len(inner.records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(inner.records))
	}
	message := inner.records[0].Message
	if !strings.Contains(message, ansiYellow+"worker"+ansiReset) {
		t.Fatalf("message = %q, want colored component prefix", message)
	}
	if !strings.Contains(message, location) {
		t.Fatalf("message = %q, want location %q", message, location)
	}
	if !strings.HasSuffix(message, "hello"+ansiReset) {
		t.Fatalf("message = %q, want ANSI reset suffix", message)
	}
	if !strings.Contains(message, ansiBlue+">"+ansiReset) {
		t.Fatalf("message = %q, want colored separator", message)
	}
}

func TestReplaceConsoleAttr(t *testing.T) {
	t.Run("component attrs are removed", func(t *testing.T) {
		// Arrange
		replaceAttr := replaceConsoleAttr(true)

		// Act
		attr := replaceAttr(nil, String("component", "api"))

		// Assert
		if !reflect.DeepEqual(attr, slog.Attr{}) {
			t.Fatalf("attr = %#v, want empty attr", attr)
		}
	})

	t.Run("level attrs use uppercase labels", func(t *testing.T) {
		// Arrange
		replaceAttr := replaceConsoleAttr(true)

		// Act
		attr := replaceAttr(nil, slog.Any(slog.LevelKey, slog.LevelWarn))

		// Assert
		if attr.Key != slog.LevelKey {
			t.Fatalf("attr.Key = %q, want %q", attr.Key, slog.LevelKey)
		}
		if attr.Value.String() != "WARN" {
			t.Fatalf("attr.Value.String() = %q, want WARN", attr.Value.String())
		}
	})

	t.Run("plain attrs gain colorized keys when colors are enabled", func(t *testing.T) {
		// Arrange
		replaceAttr := replaceConsoleAttr(false)

		// Act
		attr := replaceAttr(nil, String("requestID", "42"))

		// Assert
		if !strings.HasPrefix(attr.Key, ansiBlue) {
			t.Fatalf("attr.Key = %q, want prefix %q", attr.Key, ansiBlue)
		}
		if attr.Value.String() != "42" {
			t.Fatalf("attr.Value.String() = %q, want 42", attr.Value.String())
		}
	})
}

func TestReplaceJSONAttr(t *testing.T) {
	t.Run("level attrs use uppercase labels", func(t *testing.T) {
		// Arrange

		// Act
		attr := replaceJSONAttr(nil, slog.Any(slog.LevelKey, slog.LevelError))

		// Assert
		if attr.Value.String() != "ERROR" {
			t.Fatalf("attr.Value.String() = %q, want ERROR", attr.Value.String())
		}
	})

	t.Run("source attrs are converted to source locations", func(t *testing.T) {
		// Arrange
		source := &slog.Source{File: filepath.Join("pkg", "logger", "logger.go"), Line: 42}

		// Act
		attr := replaceJSONAttr(nil, slog.Any(slog.SourceKey, source))

		// Assert
		if attr.Value.String() != "pkg/logger/logger.go:42" {
			t.Fatalf("attr.Value.String() = %q, want %q", attr.Value.String(), "pkg/logger/logger.go:42")
		}
	})
}

func TestNewRollingLogFile(t *testing.T) {
	t.Run("returns configured rolling logger for valid path", func(t *testing.T) {
		// Arrange
		path := filepath.Join(t.TempDir(), "app.log")

		// Act
		writer := newRollingLogFile(path)

		// Assert
		if writer == nil {
			t.Fatal("newRollingLogFile() = nil, want non-nil")
		}
		if writer.Filename != path {
			t.Fatalf("Filename = %q, want %q", writer.Filename, path)
		}
		if writer.MaxSize != 64 || writer.MaxBackups != 32 || writer.MaxAge != 30 {
			t.Fatalf("unexpected lumberjack config: %+v", writer)
		}
		if !writer.Compress || !writer.LocalTime {
			t.Fatalf("writer flags = compress:%v localTime:%v, want true/true", writer.Compress, writer.LocalTime)
		}
	})

	t.Run("returns nil when parent path cannot be created", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		filePath := filepath.Join(dir, "occupied")
		if err := os.WriteFile(filePath, []byte("busy"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		invalidPath := filepath.Join(filePath, "app.log")

		// Act
		writer := newRollingLogFile(invalidPath)

		// Assert
		if writer != nil {
			t.Fatalf("newRollingLogFile() = %#v, want nil", writer)
		}
	})
}

func TestNewRootHandlerSkipsNilRollingWriter(t *testing.T) {
	t.Run("invalid file path leaves json handler disabled", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		filePath := filepath.Join(dir, "occupied")
		if err := os.WriteFile(filePath, []byte("busy"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		cfg := defaultConfig()
		cfg.stdout = os.Stdout
		cfg.filePath = filepath.Join(filePath, "app.log")
		cfg.disableFile = false
		cfg.jsonOutput = nil
		level := &slog.LevelVar{}

		// Act
		root := newRootHandler(cfg, level)
		handler, ok := root.(rootHandler)
		if !ok {
			t.Fatalf("newRootHandler() type = %T, want rootHandler", root)
		}

		// Assert
		if handler.json != nil {
			t.Fatal("json handler = non-nil, want nil when rolling file creation fails")
		}
	})

	t.Run("typed nil json output does not create json handler", func(t *testing.T) {
		// Arrange
		cfg := defaultConfig()
		cfg.stdout = os.Stdout
		cfg.disableFile = true
		var writer *lumberjack.Logger
		cfg.jsonOutput = writer
		level := &slog.LevelVar{}

		// Act
		root := newRootHandler(cfg, level)
		handler, ok := root.(rootHandler)
		if !ok {
			t.Fatalf("newRootHandler() type = %T, want rootHandler", root)
		}

		// Assert
		if handler.json != nil {
			t.Fatal("json handler = non-nil, want nil for typed nil writer")
		}
	})
}
