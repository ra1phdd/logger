package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type recordingHandler struct {
	enabled    bool
	handleErr  error
	withAttrs  []slog.Attr
	withGroups []string
	component  string
	records    []slog.Record
	contexts   []context.Context
}

func newRecordingHandler() *recordingHandler {
	return &recordingHandler{enabled: true}
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool {
	return h.enabled
}

func (h *recordingHandler) Handle(ctx context.Context, record slog.Record) error {
	h.contexts = append(h.contexts, ctx)
	h.records = append(h.records, record.Clone())
	return h.handleErr
}

func (h *recordingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	clone.withAttrs = append(clone.withAttrs, attrs...)
	return clone
}

func (h *recordingHandler) WithGroup(name string) slog.Handler {
	clone := h.clone()
	clone.withGroups = append(clone.withGroups, name)
	return clone
}

func (h *recordingHandler) WithComponent(component string) slog.Handler {
	clone := h.clone()
	clone.component = component
	return clone
}

func (h *recordingHandler) clone() *recordingHandler {
	clone := *h
	clone.withAttrs = append([]slog.Attr(nil), h.withAttrs...)
	clone.withGroups = append([]string(nil), h.withGroups...)
	clone.records = nil
	clone.contexts = nil
	return &clone
}

func newTestLogger(handler slog.Handler) *Logger {
	level := &slog.LevelVar{}
	level.Set(levelTrace)

	return &Logger{
		Logger:  slog.New(handler),
		handler: handler,
		level:   level,
		exit:    func(int) {},
	}
}

func attrByKey(record slog.Record, key string) (slog.Attr, bool) {
	var found slog.Attr
	ok := false

	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == key {
			found = attr
			ok = true
			return false
		}
		return true
	})

	return found, ok
}

func expectedLocationForTestFile(t *testing.T, file string, line int) string {
	t.Helper()

	path := file
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}

	rel, err := filepath.Rel(wd, file)
	if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		path = rel
	}

	return fmt.Sprintf("%s:%d", filepath.ToSlash(path), line)
}

func recordForPrefixTest(t *testing.T, msg string) (slog.Record, string) {
	t.Helper()

	pc, file, line, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) returned ok=false")
	}

	return slog.NewRecord(time.Now(), slog.LevelInfo, msg, pc), expectedLocationForTestFile(t, file, line)
}

func currentDefaultLogger() *Logger {
	defaultMu.RLock()
	defer defaultMu.RUnlock()

	return defaultLogger
}

func swapDefaultLogger(t *testing.T, l *Logger) {
	t.Helper()

	prev := currentDefaultLogger()
	SetDefault(l)
	t.Cleanup(func() {
		SetDefault(prev)
	})
}

func assertRecordLevel(t *testing.T, record slog.Record, wantLevel slog.Level, wantMessage string) {
	t.Helper()

	if record.Level != wantLevel {
		t.Fatalf("record.Level = %v, want %v", record.Level, wantLevel)
	}
	if record.Message != wantMessage {
		t.Fatalf("record.Message = %q, want %q", record.Message, wantMessage)
	}
}

func TestErr(t *testing.T) {
	t.Run("nil error returns empty attr", func(t *testing.T) {
		// Arrange
		var err error

		// Act
		attr := Err(err)

		// Assert
		if !reflect.DeepEqual(attr, slog.Attr{}) {
			t.Fatalf("Err(nil) = %#v, want empty attr", attr)
		}
	})

	t.Run("non nil error stores message", func(t *testing.T) {
		// Arrange
		err := errors.New("boom")

		// Act
		attr := Err(err)

		// Assert
		if attr.Key != "error" {
			t.Fatalf("attr.Key = %q, want %q", attr.Key, "error")
		}
		if attr.Value.String() != "boom" {
			t.Fatalf("attr.Value.String() = %q, want %q", attr.Value.String(), "boom")
		}
	})
}

func TestDefaultLogFilePath(t *testing.T) {
	// Arrange
	now := time.Date(2026, time.June, 11, 12, 34, 56, 123456000, time.UTC)

	// Act
	path := defaultLogFilePath(now)

	// Assert
	expected := filepath.Join(defaultLogDir, "2026-06-11 12-34-56.123456"+defaultLogFileExt)
	if path != expected {
		t.Fatalf("defaultLogFilePath() = %q, want %q", path, expected)
	}
}

func TestDefaultConfig(t *testing.T) {
	// Arrange

	// Act
	cfg := defaultConfig()

	// Assert
	if cfg.level != slog.LevelInfo {
		t.Fatalf("cfg.level = %v, want %v", cfg.level, slog.LevelInfo)
	}
	if cfg.stdout != os.Stdout {
		t.Fatalf("cfg.stdout = %v, want os.Stdout", cfg.stdout)
	}
	if !strings.HasPrefix(cfg.filePath, defaultLogDir+string(filepath.Separator)) {
		t.Fatalf("cfg.filePath = %q, want prefix %q", cfg.filePath, defaultLogDir+string(filepath.Separator))
	}
	if !strings.HasSuffix(cfg.filePath, defaultLogFileExt) {
		t.Fatalf("cfg.filePath = %q, want suffix %q", cfg.filePath, defaultLogFileExt)
	}
	if cfg.timeFormat != defaultTimeFormat {
		t.Fatalf("cfg.timeFormat = %q, want %q", cfg.timeFormat, defaultTimeFormat)
	}
	if !cfg.disableFile {
		t.Fatal("cfg.disableFile = false, want true under go test")
	}
	if cfg.exit == nil {
		t.Fatal("cfg.exit = nil, want non-nil")
	}
}

func TestOptions(t *testing.T) {
	t.Run("level string trims and ignores invalid values", func(t *testing.T) {
		// Arrange
		cfg := defaultConfig()
		cfg.level = slog.LevelWarn

		// Act
		WithLevelString(" trace ")(&cfg)
		parsedLevel := cfg.level
		WithLevelString("unknown")(&cfg)

		// Assert
		if parsedLevel != levelTrace {
			t.Fatalf("parsed level = %v, want %v", parsedLevel, levelTrace)
		}
		if cfg.level != parsedLevel {
			t.Fatalf("cfg.level after invalid input = %v, want %v", cfg.level, parsedLevel)
		}
	})

	t.Run("output option ignores nil writer", func(t *testing.T) {
		// Arrange
		cfg := defaultConfig()
		original := cfg.stdout

		// Act
		WithOutput(nil)(&cfg)

		// Assert
		if cfg.stdout != original {
			t.Fatalf("cfg.stdout changed for nil writer")
		}
	})

	t.Run("json output nil disables file output", func(t *testing.T) {
		// Arrange
		cfg := defaultConfig()

		// Act
		WithJSONOutput(nil)(&cfg)

		// Assert
		if cfg.jsonOutput != nil {
			t.Fatalf("cfg.jsonOutput = %v, want nil", cfg.jsonOutput)
		}
		if !cfg.disableFile {
			t.Fatal("cfg.disableFile = false, want true")
		}
	})

	t.Run("file options toggle file output", func(t *testing.T) {
		// Arrange
		cfg := defaultConfig()
		WithoutFile()(&cfg)

		// Act
		WithFile("custom.log")(&cfg)

		// Assert
		if cfg.filePath != "custom.log" {
			t.Fatalf("cfg.filePath = %q, want %q", cfg.filePath, "custom.log")
		}
		if cfg.disableFile {
			t.Fatal("cfg.disableFile = true, want false")
		}
	})

	t.Run("formatting options trim and ignore empty values", func(t *testing.T) {
		// Arrange
		cfg := defaultConfig()
		exitCalls := 0
		exitFn := func(int) {
			exitCalls++
		}

		// Act
		WithComponent(" api ")(&cfg)
		WithTimeFormat("")(&cfg)
		WithNoColor(true)(&cfg)
		WithExit(nil)(&cfg)
		WithExit(exitFn)(&cfg)

		// Assert
		if cfg.component != "api" {
			t.Fatalf("cfg.component = %q, want %q", cfg.component, "api")
		}
		if cfg.timeFormat != defaultTimeFormat {
			t.Fatalf("cfg.timeFormat = %q, want %q", cfg.timeFormat, defaultTimeFormat)
		}
		if !cfg.noColor {
			t.Fatal("cfg.noColor = false, want true")
		}
		if cfg.exit == nil {
			t.Fatal("cfg.exit = nil, want non-nil")
		}

		cfg.exit(1)
		if exitCalls != 1 {
			t.Fatalf("exitCalls = %d, want 1", exitCalls)
		}
	})
}

func TestLevelHelpers(t *testing.T) {
	t.Run("parse level supports aliases and invalid fallback", func(t *testing.T) {
		// Arrange

		// Act
		warnLevel, warnOK := parseLevel(" Warning ")
		fatalLevel, fatalOK := parseLevel("fatal")
		invalidLevel, invalidOK := parseLevel("unknown")

		// Assert
		if warnLevel != slog.LevelWarn || !warnOK {
			t.Fatalf("parseLevel(warning) = (%v, %v), want (%v, true)", warnLevel, warnOK, slog.LevelWarn)
		}
		if fatalLevel != levelFatal || !fatalOK {
			t.Fatalf("parseLevel(fatal) = (%v, %v), want (%v, true)", fatalLevel, fatalOK, levelFatal)
		}
		if invalidLevel != slog.LevelInfo || invalidOK {
			t.Fatalf("parseLevel(unknown) = (%v, %v), want (%v, false)", invalidLevel, invalidOK, slog.LevelInfo)
		}
	})

	t.Run("level string maps boundaries", func(t *testing.T) {
		// Arrange

		// Act
		traceValue := levelString(levelTrace)
		debugValue := levelString(slog.LevelDebug)
		fatalValue := levelString(levelFatal)

		// Assert
		if traceValue != "trace" {
			t.Fatalf("levelString(trace) = %q, want trace", traceValue)
		}
		if debugValue != "debug" {
			t.Fatalf("levelString(debug) = %q, want debug", debugValue)
		}
		if fatalValue != "fatal" {
			t.Fatalf("levelString(fatal) = %q, want fatal", fatalValue)
		}
	})

	t.Run("level label maps boundaries", func(t *testing.T) {
		// Arrange

		// Act
		traceLabel := levelLabel(levelTrace)
		warnLabel := levelLabel(slog.LevelWarn)
		errorLabel := levelLabel(slog.LevelError)

		// Assert
		if traceLabel != "TRACE" {
			t.Fatalf("levelLabel(trace) = %q, want TRACE", traceLabel)
		}
		if warnLabel != "WARN" {
			t.Fatalf("levelLabel(warn) = %q, want WARN", warnLabel)
		}
		if errorLabel != "ERROR" {
			t.Fatalf("levelLabel(error) = %q, want ERROR", errorLabel)
		}
	})

	t.Run("level color maps boundaries", func(t *testing.T) {
		// Arrange

		// Act
		traceColor := levelColor(levelTrace)
		infoColor := levelColor(slog.LevelInfo)
		fatalColor := levelColor(levelFatal)

		// Assert
		if traceColor != 13 {
			t.Fatalf("levelColor(trace) = %d, want 13", traceColor)
		}
		if infoColor != 10 {
			t.Fatalf("levelColor(info) = %d, want 10", infoColor)
		}
		if fatalColor != 13 {
			t.Fatalf("levelColor(fatal) = %d, want 13", fatalColor)
		}
	})
}

func TestSourceHelpers(t *testing.T) {
	t.Run("zero pc returns empty source", func(t *testing.T) {
		// Arrange

		// Act
		source := sourceFromPC(0)

		// Assert
		if source != (slog.Source{}) {
			t.Fatalf("sourceFromPC(0) = %#v, want empty source", source)
		}
	})

	t.Run("empty file values fall back to unknown", func(t *testing.T) {
		// Arrange

		// Act
		location := sourceLocation("", 10)
		component := componentFromPath("")

		// Assert
		if location != "unknown" {
			t.Fatalf("sourceLocation(empty) = %q, want unknown", location)
		}
		if component != "unknown" {
			t.Fatalf("componentFromPath(empty) = %q, want unknown", component)
		}
	})

	t.Run("logger source detection uses package path", func(t *testing.T) {
		// Arrange

		// Act
		loggerPath := isLoggerSource("D:/repo/pkg/logger/logger.go")
		otherPath := isLoggerSource("D:/repo/internal/app/service.go")

		// Assert
		if !loggerPath {
			t.Fatal("isLoggerSource() = false, want true for logger path")
		}
		if otherPath {
			t.Fatal("isLoggerSource() = true, want false for non-logger path")
		}
	})
}

func TestNewCreatesConfiguredLogger(t *testing.T) {
	// Arrange
	exitCalls := 0
	log := New(
		WithLevel(levelTrace),
		WithOutput(io.Discard),
		WithoutFile(),
		WithComponent(" api "),
		WithExit(func(int) { exitCalls++ }),
	)

	// Act
	enabled := log.Enabled(context.Background(), levelTrace)

	// Assert
	if !enabled {
		t.Fatal("Enabled(trace) = false, want true")
	}
	if log.Level() != levelTrace {
		t.Fatalf("Level() = %v, want %v", log.Level(), levelTrace)
	}
	if log.component != "api" {
		t.Fatalf("component = %q, want %q", log.component, "api")
	}
	log.exit(1)
	if exitCalls != 1 {
		t.Fatalf("exitCalls = %d, want 1", exitCalls)
	}
}

func TestInitReplacesDefaultLogger(t *testing.T) {
	// Arrange
	prev := currentDefaultLogger()
	t.Cleanup(func() {
		SetDefault(prev)
	})

	// Act
	Init(WithLevel(slog.LevelDebug), WithOutput(io.Discard), WithoutFile())

	// Assert
	if Default().Level() != slog.LevelDebug {
		t.Fatalf("Default().Level() = %v, want %v", Default().Level(), slog.LevelDebug)
	}
}

func TestLoggerWithCreatesChildWithAttrs(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	log := newTestLogger(handler)

	// Act
	child := log.With(String("requestID", "42"))
	child.Info("hello")

	// Assert
	childHandler, ok := child.handler.(*recordingHandler)
	if !ok {
		t.Fatalf("child.handler type = %T, want *recordingHandler", child.handler)
	}
	if len(childHandler.withAttrs) != 1 {
		t.Fatalf("len(withAttrs) = %d, want 1", len(childHandler.withAttrs))
	}
	if childHandler.withAttrs[0].Key != "requestID" {
		t.Fatalf("withAttrs[0].Key = %q, want requestID", childHandler.withAttrs[0].Key)
	}
	if len(childHandler.records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(childHandler.records))
	}
	if len(handler.records) != 0 {
		t.Fatalf("parent handler recorded %d messages, want 0", len(handler.records))
	}
}

func TestLoggerWithGroupCreatesChildWithGroup(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	log := newTestLogger(handler)

	// Act
	child := log.WithGroup("http")
	child.Info("hello")

	// Assert
	childHandler, ok := child.handler.(*recordingHandler)
	if !ok {
		t.Fatalf("child.handler type = %T, want *recordingHandler", child.handler)
	}
	if len(childHandler.withGroups) != 1 {
		t.Fatalf("len(withGroups) = %d, want 1", len(childHandler.withGroups))
	}
	if childHandler.withGroups[0] != "http" {
		t.Fatalf("withGroups[0] = %q, want http", childHandler.withGroups[0])
	}
	if len(childHandler.records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(childHandler.records))
	}
}

func TestLoggerNamedCreatesComponentScopedChild(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	log := newTestLogger(handler)

	// Act
	child := log.Named(" api ")

	// Assert
	childHandler, ok := child.handler.(*recordingHandler)
	if !ok {
		t.Fatalf("child.handler type = %T, want *recordingHandler", child.handler)
	}
	if child.component != "api" {
		t.Fatalf("child.component = %q, want api", child.component)
	}
	if childHandler.component != "api" {
		t.Fatalf("handler.component = %q, want api", childHandler.component)
	}
	if log.Named("   ") != log {
		t.Fatal("Named(blank) did not return the original logger")
	}
}

func TestLoggerLogUsesBackgroundContextWhenNil(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	log := newTestLogger(handler)

	// Act
	log.Log(nil, slog.LevelInfo, "hello", String("requestID", "42"))

	// Assert
	if len(handler.records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(handler.records))
	}
	if handler.contexts[0] == nil {
		t.Fatal("context = nil, want background context")
	}
	if handler.records[0].Message != "hello" {
		t.Fatalf("message = %q, want hello", handler.records[0].Message)
	}
	attr, ok := attrByKey(handler.records[0], "requestID")
	if !ok {
		t.Fatal("requestID attr missing")
	}
	if attr.Value.String() != "42" {
		t.Fatalf("requestID attr = %q, want 42", attr.Value.String())
	}
}

func TestLoggerLogSkipsDisabledLevels(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	handler.enabled = false
	log := newTestLogger(handler)

	// Act
	log.Log(context.Background(), slog.LevelInfo, "hello")

	// Assert
	if len(handler.records) != 0 {
		t.Fatalf("len(records) = %d, want 0", len(handler.records))
	}
}

func TestLoggerSetLogLevelFallsBackToInfo(t *testing.T) {
	// Arrange
	log := newTestLogger(newRecordingHandler())
	log.SetLevel(levelTrace)

	// Act
	log.SetLogLevel("not-a-level")

	// Assert
	if log.Level() != slog.LevelInfo {
		t.Fatalf("Level() = %v, want %v", log.Level(), slog.LevelInfo)
	}
	if log.GetLogLevel() != "info" {
		t.Fatalf("GetLogLevel() = %q, want info", log.GetLogLevel())
	}
}

func TestLoggerFatalLogsAndExits(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	exitCode := 0
	log := newTestLogger(handler)
	log.exit = func(code int) {
		exitCode = code
	}

	// Act
	log.Fatal("boom")

	// Assert
	if len(handler.records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(handler.records))
	}
	if handler.records[0].Level != levelFatal {
		t.Fatalf("record level = %v, want %v", handler.records[0].Level, levelFatal)
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
}

func TestDefaultHelpersUseDefaultLogger(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	log := newTestLogger(handler)
	swapDefaultLogger(t, log)

	// Act
	SetLogLevel("debug")
	Info("hello", String("scope", "default"))

	// Assert
	if GetLogLevel() != "debug" {
		t.Fatalf("GetLogLevel() = %q, want debug", GetLogLevel())
	}
	if len(handler.records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(handler.records))
	}
	if handler.records[0].Message != "hello" {
		t.Fatalf("message = %q, want hello", handler.records[0].Message)
	}
	attr, ok := attrByKey(handler.records[0], "scope")
	if !ok {
		t.Fatal("scope attr missing")
	}
	if attr.Value.String() != "default" {
		t.Fatalf("scope attr = %q, want default", attr.Value.String())
	}
}

func TestDefaultNamedAndWithReturnChildLoggers(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	log := newTestLogger(handler)
	swapDefaultLogger(t, log)

	// Act
	named := Named(" api ")
	withAttrs := With(String("scope", "pkg"))
	withAttrs.Info("hello")

	// Assert
	if named.component != "api" {
		t.Fatalf("named.component = %q, want api", named.component)
	}
	childHandler, ok := withAttrs.handler.(*recordingHandler)
	if !ok {
		t.Fatalf("withAttrs.handler type = %T, want *recordingHandler", withAttrs.handler)
	}
	if len(childHandler.records) != 1 {
		t.Fatalf("len(child records) = %d, want 1", len(childHandler.records))
	}
	if len(childHandler.withAttrs) != 1 {
		t.Fatalf("len(withAttrs) = %d, want 1", len(childHandler.withAttrs))
	}
	if childHandler.withAttrs[0].Key != "scope" {
		t.Fatalf("withAttrs[0].Key = %q, want scope", childHandler.withAttrs[0].Key)
	}
	if childHandler.withAttrs[0].Value.String() != "pkg" {
		t.Fatalf("scope attr = %q, want pkg", childHandler.withAttrs[0].Value.String())
	}
}

func TestContextHelpersUseContextLogger(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	log := newTestLogger(handler)
	ctx := NewContext(context.Background(), log)

	// Act
	InfoContext(ctx, "hello", String("scope", "context"))

	// Assert
	if FromContext(ctx) != log {
		t.Fatal("FromContext() did not return the logger from context")
	}
	if len(handler.records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(handler.records))
	}
	if handler.contexts[0] != ctx {
		t.Fatal("handler context does not match the original context")
	}
	attr, ok := attrByKey(handler.records[0], "scope")
	if !ok {
		t.Fatal("scope attr missing")
	}
	if attr.Value.String() != "context" {
		t.Fatalf("scope attr = %q, want context", attr.Value.String())
	}
}

func TestDefaultContextWrappersUseLoggerFromContext(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	log := newTestLogger(handler)
	exitCalls := 0
	log.exit = func(int) {
		exitCalls++
	}
	ctx := NewContext(context.Background(), log)

	// Act
	TraceContext(ctx, "trace")
	DebugContext(ctx, "debug")
	WarnContext(ctx, "warn")
	ErrorContext(ctx, "error")
	FatalContext(ctx, "fatal")

	// Assert
	if len(handler.records) != 5 {
		t.Fatalf("len(records) = %d, want 5", len(handler.records))
	}
	assertRecordLevel(t, handler.records[0], levelTrace, "trace")
	assertRecordLevel(t, handler.records[1], slog.LevelDebug, "debug")
	assertRecordLevel(t, handler.records[2], slog.LevelWarn, "warn")
	assertRecordLevel(t, handler.records[3], slog.LevelError, "error")
	assertRecordLevel(t, handler.records[4], levelFatal, "fatal")
	if exitCalls != 1 {
		t.Fatalf("exitCalls = %d, want 1", exitCalls)
	}
}

func TestNewContextFallsBackToDefaultLogger(t *testing.T) {
	// Arrange
	log := newTestLogger(newRecordingHandler())
	swapDefaultLogger(t, log)

	// Act
	ctx := NewContext(nil, nil)

	// Assert
	if ctx == nil {
		t.Fatal("NewContext(nil, nil) = nil, want non-nil context")
	}
	if FromContext(ctx) != log {
		t.Fatal("FromContext() did not return the default logger")
	}
}

func TestFromContextNilFallsBackToDefault(t *testing.T) {
	// Arrange
	log := newTestLogger(newRecordingHandler())
	swapDefaultLogger(t, log)

	// Act
	fromContext := FromContext(nil)

	// Assert
	if fromContext != log {
		t.Fatal("FromContext(nil) did not return the default logger")
	}
}

func TestNilLoggerFallbacksAreSafe(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	defaultLog := newTestLogger(handler)
	swapDefaultLogger(t, defaultLog)
	var log *Logger

	// Act
	child := log.With(String("scope", "nil"))
	child.Info("hello")

	// Assert
	if child == nil {
		t.Fatal("With() on nil logger returned nil")
	}
	childHandler, ok := child.handler.(*recordingHandler)
	if !ok {
		t.Fatalf("child.handler type = %T, want *recordingHandler", child.handler)
	}
	if log.Slog() == nil {
		t.Fatal("Slog() on nil logger returned nil")
	}
	if log.Handler() == nil {
		t.Fatal("Handler() on nil logger returned nil")
	}
	if log.Level() != slog.LevelInfo {
		t.Fatalf("Level() on nil logger = %v, want %v", log.Level(), slog.LevelInfo)
	}
	if log.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("Enabled() on nil logger = true, want false")
	}
	if len(childHandler.records) != 1 {
		t.Fatalf("len(child records) = %d, want 1", len(childHandler.records))
	}
	if len(handler.records) != 0 {
		t.Fatalf("len(parent records) = %d, want 0", len(handler.records))
	}
}

func TestNilLoggerLogIsNoop(t *testing.T) {
	// Arrange
	var log *Logger

	// Act
	log.Log(context.Background(), slog.LevelInfo, "hello")

	// Assert
	if log != nil {
		t.Fatal("nil logger changed unexpectedly")
	}
}

func TestLoggerLevelMethodsLogExpectedLevels(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	log := newTestLogger(handler)
	exitCalls := 0
	log.exit = func(int) {
		exitCalls++
	}

	// Act
	log.Trace("trace")
	log.Debug("debug")
	log.Info("info")
	log.Warn("warn")
	log.Error("error")
	log.Fatal("fatal")

	// Assert
	if len(handler.records) != 6 {
		t.Fatalf("len(records) = %d, want 6", len(handler.records))
	}
	assertRecordLevel(t, handler.records[0], levelTrace, "trace")
	assertRecordLevel(t, handler.records[1], slog.LevelDebug, "debug")
	assertRecordLevel(t, handler.records[2], slog.LevelInfo, "info")
	assertRecordLevel(t, handler.records[3], slog.LevelWarn, "warn")
	assertRecordLevel(t, handler.records[4], slog.LevelError, "error")
	assertRecordLevel(t, handler.records[5], levelFatal, "fatal")
	if exitCalls != 1 {
		t.Fatalf("exitCalls = %d, want 1", exitCalls)
	}
}

func TestLoggerContextMethodsLogExpectedLevels(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	log := newTestLogger(handler)
	exitCalls := 0
	log.exit = func(int) {
		exitCalls++
	}
	ctx := context.WithValue(context.Background(), "requestID", "42")

	// Act
	log.TraceContext(ctx, "trace")
	log.DebugContext(ctx, "debug")
	log.InfoContext(ctx, "info")
	log.WarnContext(ctx, "warn")
	log.ErrorContext(ctx, "error")
	log.FatalContext(ctx, "fatal")

	// Assert
	if len(handler.records) != 6 {
		t.Fatalf("len(records) = %d, want 6", len(handler.records))
	}
	if handler.contexts[0] != ctx || handler.contexts[5] != ctx {
		t.Fatal("context methods did not pass through the provided context")
	}
	assertRecordLevel(t, handler.records[0], levelTrace, "trace")
	assertRecordLevel(t, handler.records[1], slog.LevelDebug, "debug")
	assertRecordLevel(t, handler.records[2], slog.LevelInfo, "info")
	assertRecordLevel(t, handler.records[3], slog.LevelWarn, "warn")
	assertRecordLevel(t, handler.records[4], slog.LevelError, "error")
	assertRecordLevel(t, handler.records[5], levelFatal, "fatal")
	if exitCalls != 1 {
		t.Fatalf("exitCalls = %d, want 1", exitCalls)
	}
}

func TestDefaultLevelWrappersLogExpectedLevels(t *testing.T) {
	// Arrange
	handler := newRecordingHandler()
	log := newTestLogger(handler)
	exitCalls := 0
	log.exit = func(int) {
		exitCalls++
	}
	swapDefaultLogger(t, log)

	// Act
	Trace("trace")
	Debug("debug")
	Warn("warn")
	Error("error")
	Fatal("fatal")

	// Assert
	if len(handler.records) != 5 {
		t.Fatalf("len(records) = %d, want 5", len(handler.records))
	}
	assertRecordLevel(t, handler.records[0], levelTrace, "trace")
	assertRecordLevel(t, handler.records[1], slog.LevelDebug, "debug")
	assertRecordLevel(t, handler.records[2], slog.LevelWarn, "warn")
	assertRecordLevel(t, handler.records[3], slog.LevelError, "error")
	assertRecordLevel(t, handler.records[4], levelFatal, "fatal")
	if exitCalls != 1 {
		t.Fatalf("exitCalls = %d, want 1", exitCalls)
	}
}
