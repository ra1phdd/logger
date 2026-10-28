package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

type Option func(*config)

type config struct {
	level           slog.Level
	stdout          io.Writer
	jsonOutput      io.Writer
	filePath        string
	component       string
	timeFormat      string
	noColor         bool
	disableFile     bool
	consoleDisabled bool
	consoleLevel    *slog.LevelVar
	jsonLevel       *slog.LevelVar
	exit            func(int)
	victorialogs    *VictoriaLogsConfig
	loki            *LokiConfig
}

func defaultConfig() config {
	return config{
		level:           slog.LevelInfo,
		stdout:          os.Stdout,
		filePath:        defaultLogFilePath(time.Now()),
		timeFormat:      defaultTimeFormat,
		disableFile:     runningUnderGoTest(),
		exit:            os.Exit,
		consoleLevel:    nil,
		jsonLevel:       nil,
		consoleDisabled: false,
	}
}

func defaultLogFilePath(now time.Time) string {
	return filepath.Join(defaultLogDir, now.Format(defaultLogFilePattern)+defaultLogFileExt)
}

func runningUnderGoTest() bool {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test.") {
			return true
		}
	}

	return strings.Contains(filepath.Base(os.Args[0]), ".test")
}

func WithLevel(level slog.Level) Option {
	return func(cfg *config) {
		cfg.level = level
	}
}

func WithLevelString(level string) Option {
	return func(cfg *config) {
		if parsed, ok := parseLevel(level); ok {
			cfg.level = parsed
		}
	}
}

func WithOutput(w io.Writer) Option {
	return func(cfg *config) {
		if writer := normalizeWriter(w); writer != nil {
			cfg.stdout = writer
		}
	}
}

func WithJSONOutput(w io.Writer) Option {
	return func(cfg *config) {
		cfg.jsonOutput = normalizeWriter(w)
		cfg.disableFile = cfg.jsonOutput == nil
	}
}

func WithFile(path string) Option {
	return func(cfg *config) {
		if path != "" {
			cfg.filePath = path
			cfg.disableFile = false
		}
	}
}

func WithoutFile() Option {
	return func(cfg *config) {
		cfg.jsonOutput = nil
		cfg.disableFile = true
	}
}

func WithComponent(component string) Option {
	return func(cfg *config) {
		cfg.component = strings.TrimSpace(component)
	}
}

func WithTimeFormat(format string) Option {
	return func(cfg *config) {
		if format != "" {
			cfg.timeFormat = format
		}
	}
}

func WithNoColor(noColor bool) Option {
	return func(cfg *config) {
		cfg.noColor = noColor
	}
}

func WithExit(exit func(int)) Option {
	return func(cfg *config) {
		if exit != nil {
			cfg.exit = exit
		}
	}
}

// DeliveryConfig holds common delivery settings for HTTP sinks.
type DeliveryConfig struct {
	BufferSize       int
	BatchSize        int
	FlushInterval    time.Duration
	MaxRetries       int
	ShutdownTimeout  time.Duration
	RetryInitialWait time.Duration
	RetryMaxWait     time.Duration
}

func (d *DeliveryConfig) setDefaults() {
	if d.BufferSize <= 0 {
		d.BufferSize = defaultBufferSize
	}
	if d.BatchSize <= 0 {
		d.BatchSize = defaultBatchSize
	}
	if d.FlushInterval <= 0 {
		d.FlushInterval = defaultFlushInterval
	}
	if d.MaxRetries <= 0 {
		d.MaxRetries = defaultMaxRetries
	}
	if d.ShutdownTimeout <= 0 {
		d.ShutdownTimeout = defaultShutdownTimeout
	}
	if d.RetryInitialWait <= 0 {
		d.RetryInitialWait = defaultRetryInitial
	}
	if d.RetryMaxWait <= 0 {
		d.RetryMaxWait = defaultRetryMax
	}
}

// VictoriaLogsConfig configures the VictoriaLogs sink.
type VictoriaLogsConfig struct {
	URL          string
	StreamFields []string
	Delivery     DeliveryConfig
	StaticLabels map[string]string
}

// LokiConfig configures the Loki sink.
type LokiConfig struct {
	URL          string
	Labels       []string
	Delivery     DeliveryConfig
	StaticLabels map[string]string
}

func WithConsoleLevel(level slog.Level) Option {
	return func(cfg *config) {
		if cfg.consoleLevel == nil {
			cfg.consoleLevel = &slog.LevelVar{}
		}
		cfg.consoleLevel.Set(level)
	}
}

func WithConsoleLevelString(level string) Option {
	return func(cfg *config) {
		if parsed, ok := parseLevel(level); ok {
			if cfg.consoleLevel == nil {
				cfg.consoleLevel = &slog.LevelVar{}
			}
			cfg.consoleLevel.Set(parsed)
		}
	}
}

func WithJSONLevel(level slog.Level) Option {
	return func(cfg *config) {
		if cfg.jsonLevel == nil {
			cfg.jsonLevel = &slog.LevelVar{}
		}
		cfg.jsonLevel.Set(level)
	}
}

func WithJSONLevelString(level string) Option {
	return func(cfg *config) {
		if parsed, ok := parseLevel(level); ok {
			if cfg.jsonLevel == nil {
				cfg.jsonLevel = &slog.LevelVar{}
			}
			cfg.jsonLevel.Set(parsed)
		}
	}
}

func WithoutConsole() Option {
	return func(cfg *config) {
		cfg.consoleDisabled = true
	}
}

func WithVictoriaLogs(vlCfg VictoriaLogsConfig) Option {
	return func(cfg *config) {
		cfg.victorialogs = &vlCfg
		cfg.victorialogs.Delivery.setDefaults()
		if len(cfg.victorialogs.StreamFields) == 0 {
			cfg.victorialogs.StreamFields = []string{"component", "level"}
		}
	}
}

func WithLoki(lokiCfg LokiConfig) Option {
	return func(cfg *config) {
		cfg.loki = &lokiCfg
		cfg.loki.Delivery.setDefaults()
		if len(cfg.loki.Labels) == 0 {
			cfg.loki.Labels = []string{"component", "level"}
		}
	}
}

func normalizeWriter(w io.Writer) io.Writer {
	if w == nil {
		return nil
	}

	v := reflect.ValueOf(w)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return nil
		}
	}

	return w
}
