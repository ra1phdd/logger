package logger

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-colorable"
	"gopkg.in/natefinch/lumberjack.v2"
)

func newRootHandler(cfg config, level *slog.LevelVar, sinkGrp *sinkGroup) slog.Handler {
	handler := &rootHandler{
		component: cfg.component,
		level:     level,
	}

	// Build console sink unless disabled
	if !cfg.consoleDisabled {
		consoleOutput := normalizeWriter(cfg.stdout)
		if consoleOutput == nil {
			consoleOutput = os.Stdout
		}
		if file, ok := cfg.stdout.(*os.File); ok {
			consoleOutput = colorable.NewColorable(file)
		}

		baseTintHandler := tint.NewHandler(consoleOutput, &tint.Options{
			Level:       level,
			TimeFormat:  cfg.timeFormat,
			NoColor:     cfg.noColor,
			ReplaceAttr: replaceConsoleAttr(cfg.noColor),
		})

		consolePrefixH := &consolePrefixHandler{
			Handler:   baseTintHandler,
			component: cfg.component,
			noColor:   cfg.noColor,
		}

		filteredConsole := &levelFilterHandler{
			handler:     consolePrefixH,
			globalLevel: level,
			sinkLevel:   cfg.consoleLevel,
		}

		// Wrap in async sink to avoid blocking
		asyncConsole := newAsyncSink(filteredConsole, defaultBufferSize, sinkGrp)

		handler.consoleSink = asyncConsole
		sinkGrp.add(sink{
			name:    "console",
			handler: asyncConsole,
			stats:   asyncConsole.stats,
		})
	}

	// Build JSON file sink unless disabled
	if !cfg.disableFile {
		jsonOutput := normalizeWriter(cfg.jsonOutput)
		if jsonOutput == nil {
			fileOutput := newRollingLogFile(cfg.filePath)
			if fileOutput != nil {
				jsonOutput = fileOutput
			} else {
				_, _ = os.Stderr.WriteString("WARNING: Failed to create log file at " + cfg.filePath + ", logging to console only\n")
			}
		}

		if jsonOutput != nil {
			baseJSONHandler := slog.NewJSONHandler(jsonOutput, &slog.HandlerOptions{
				AddSource:   true,
				Level:       level,
				ReplaceAttr: replaceJSONAttr,
			})

			filteredJSON := &levelFilterHandler{
				handler:     baseJSONHandler,
				globalLevel: level,
				sinkLevel:   cfg.jsonLevel,
			}

			// Wrap in async sink to avoid blocking
			asyncJSON := newAsyncSink(filteredJSON, defaultBufferSize, sinkGrp)

			handler.jsonSink = asyncJSON
			sinkGrp.add(sink{
				name:    "json_file",
				handler: asyncJSON,
				stats:   asyncJSON.stats,
			})
		}
	}

	// Build VictoriaLogs sink if configured
	if cfg.victorialogs != nil {
		vlSink := newVictoriaLogsSink(cfg.victorialogs, level, sinkGrp)
		handler.vlSink = vlSink
		sinkGrp.add(sink{
			name:    "victorialogs",
			handler: vlSink.handler,
			stats:   vlSink.stats,
		})
	}

	// Build Loki sink if configured
	if cfg.loki != nil {
		lokiSink := newLokiSink(cfg.loki, level, sinkGrp)
		handler.lokiSink = lokiSink
		sinkGrp.add(sink{
			name:    "loki",
			handler: lokiSink.handler,
			stats:   lokiSink.stats,
		})
	}

	return handler
}

func newRollingLogFile(path string) *lumberjack.Logger {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil
		}
	}

	return &lumberjack.Logger{
		Filename:   path,
		MaxSize:    64,
		MaxBackups: 32,
		MaxAge:     30,
		Compress:   true,
		LocalTime:  true,
	}
}

type rootHandler struct {
	consoleSink slog.Handler
	jsonSink    slog.Handler
	vlSink      *httpSink
	lokiSink    *httpSink
	component   string
	level       *slog.LevelVar
}

func (h *rootHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.consoleSink != nil && h.consoleSink.Enabled(ctx, level) {
		return true
	}
	if h.jsonSink != nil && h.jsonSink.Enabled(ctx, level) {
		return true
	}
	if h.vlSink != nil && h.vlSink.handler.Enabled(ctx, level) {
		return true
	}
	if h.lokiSink != nil && h.lokiSink.handler.Enabled(ctx, level) {
		return true
	}
	return false
}

func (h *rootHandler) Handle(ctx context.Context, record slog.Record) error {
	var result error
	record.PC = projectCallerPC(record.PC)

	if h.consoleSink != nil {
		if err := h.consoleSink.Handle(ctx, record.Clone()); err != nil {
			result = errors.Join(result, err)
		}
	}

	if h.jsonSink != nil {
		if err := h.jsonSink.Handle(ctx, record.Clone()); err != nil {
			result = errors.Join(result, err)
		}
	}

	if h.vlSink != nil {
		if err := h.vlSink.Handle(ctx, record.Clone()); err != nil {
			result = errors.Join(result, err)
		}
	}

	if h.lokiSink != nil {
		if err := h.lokiSink.Handle(ctx, record.Clone()); err != nil {
			result = errors.Join(result, err)
		}
	}

	return result
}

func (h *rootHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var consoleSink, jsonSink slog.Handler
	var vlSink, lokiSink *httpSink

	if h.consoleSink != nil {
		consoleSink = h.consoleSink.WithAttrs(attrs)
	}
	if h.jsonSink != nil {
		jsonSink = h.jsonSink.WithAttrs(attrs)
	}
	if h.vlSink != nil {
		vlSink = h.vlSink.WithAttrs(attrs)
	}
	if h.lokiSink != nil {
		lokiSink = h.lokiSink.WithAttrs(attrs)
	}

	return &rootHandler{
		consoleSink: consoleSink,
		jsonSink:    jsonSink,
		vlSink:      vlSink,
		lokiSink:    lokiSink,
		component:   h.component,
		level:       h.level,
	}
}

func (h *rootHandler) WithGroup(name string) slog.Handler {
	var consoleSink, jsonSink slog.Handler
	var vlSink, lokiSink *httpSink

	if h.consoleSink != nil {
		consoleSink = h.consoleSink.WithGroup(name)
	}
	if h.jsonSink != nil {
		jsonSink = h.jsonSink.WithGroup(name)
	}
	if h.vlSink != nil {
		vlSink = h.vlSink.WithGroup(name)
	}
	if h.lokiSink != nil {
		lokiSink = h.lokiSink.WithGroup(name)
	}

	return &rootHandler{
		consoleSink: consoleSink,
		jsonSink:    jsonSink,
		vlSink:      vlSink,
		lokiSink:    lokiSink,
		component:   h.component,
		level:       h.level,
	}
}

func (h *rootHandler) WithComponent(component string) slog.Handler {
	var consoleSink = h.consoleSink
	if withComponentHandler, ok := consoleSink.(interface{ WithComponent(component string) slog.Handler }); ok {
		consoleSink = withComponentHandler.WithComponent(component)
	}

	return &rootHandler{
		consoleSink: consoleSink,
		jsonSink:    h.jsonSink,
		vlSink:      h.vlSink,
		lokiSink:    h.lokiSink,
		component:   component,
		level:       h.level,
	}
}

type consolePrefixHandler struct {
	slog.Handler

	component string
	noColor   bool
}

func (h consolePrefixHandler) WithComponent(component string) slog.Handler {
	h.component = component
	return h
}

type prefixCacheKey struct {
	pc        uintptr
	component string
	noColor   bool
}

var consolePrefixCache sync.Map

func (h consolePrefixHandler) Handle(ctx context.Context, record slog.Record) error {
	record.Message = h.prefix(record.PC, h.component) + record.Message
	if !h.noColor {
		record.Message += ansiReset
	}

	return h.Handler.Handle(ctx, record)
}

func (h consolePrefixHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return consolePrefixHandler{
		Handler:   h.Handler.WithAttrs(attrs),
		component: h.component,
		noColor:   h.noColor,
	}
}

func (h consolePrefixHandler) WithGroup(name string) slog.Handler {
	return consolePrefixHandler{
		Handler:   h.Handler.WithGroup(name),
		component: h.component,
		noColor:   h.noColor,
	}
}

func (h consolePrefixHandler) prefix(pc uintptr, component string) string {
	cacheKey := prefixCacheKey{
		pc:        pc,
		component: component,
		noColor:   h.noColor,
	}
	if cached, ok := consolePrefixCache.Load(cacheKey); ok {
		return cached.(string)
	}

	source := sourceFromPC(pc)
	if component == "" {
		component = componentFromPath(source.File)
	}
	location := sourceLocation(source.File, source.Line)

	if h.noColor {
		prefix := component + " " + location + " > "
		consolePrefixCache.Store(cacheKey, prefix)
		return prefix
	}

	prefix := ansiYellow + component + ansiReset + " " +
		ansiWhite + location + ansiReset + " " +
		ansiBlue + ">" + ansiReset + " " +
		ansiWhite
	consolePrefixCache.Store(cacheKey, prefix)
	return prefix
}

func replaceConsoleAttr(noColor bool) func([]string, slog.Attr) slog.Attr {
	return func(_ []string, attr slog.Attr) slog.Attr {
		switch attr.Key {
		case "component":
			return slog.Attr{}
		case slog.TimeKey:
			return tint.Attr(8, attr)
		case slog.LevelKey:
			level, _ := attr.Value.Any().(slog.Level)
			return tint.Attr(levelColor(level), slog.String(attr.Key, levelLabel(level)))
		case slog.MessageKey:
			return attr
		default:
			if !noColor {
				attr.Key = ansiBlue + attr.Key
			}
			return attr
		}
	}
}

func replaceJSONAttr(_ []string, attr slog.Attr) slog.Attr {
	switch attr.Key {
	case slog.LevelKey:
		if level, ok := attr.Value.Any().(slog.Level); ok {
			return slog.String(attr.Key, levelLabel(level))
		}
	case slog.SourceKey:
		if source, ok := attr.Value.Any().(*slog.Source); ok {
			return slog.String(attr.Key, sourceLocation(source.File, source.Line))
		}
	}
	return attr
}
