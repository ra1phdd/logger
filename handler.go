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

func newRootHandler(cfg config, level *slog.LevelVar) slog.Handler {
	consoleOutput := cfg.stdout
	if file, ok := cfg.stdout.(*os.File); ok {
		consoleOutput = colorable.NewColorable(file)
	}

	consoleHandler := consolePrefixHandler{
		Handler: tint.NewHandler(consoleOutput, &tint.Options{
			Level:       level,
			TimeFormat:  cfg.timeFormat,
			NoColor:     cfg.noColor,
			ReplaceAttr: replaceConsoleAttr(cfg.noColor),
		}),
		component: cfg.component,
		noColor:   cfg.noColor,
	}

	jsonOutput := cfg.jsonOutput
	if jsonOutput == nil && !cfg.disableFile {
		jsonOutput = newRollingLogFile(cfg.filePath)
	}

	handler := rootHandler{
		console:   consoleHandler,
		component: cfg.component,
	}
	if jsonOutput != nil {
		handler.json = slog.NewJSONHandler(jsonOutput, &slog.HandlerOptions{
			AddSource:   true,
			Level:       level,
			ReplaceAttr: replaceJSONAttr,
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
	console   slog.Handler
	json      slog.Handler
	component string
}

func (h rootHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.console != nil && h.console.Enabled(ctx, level) {
		return true
	}
	return h.json != nil && h.json.Enabled(ctx, level)
}

func (h rootHandler) Handle(ctx context.Context, record slog.Record) error {
	var result error

	if h.console != nil {
		if err := h.console.Handle(ctx, record); err != nil {
			result = errors.Join(result, err)
		}
	}

	if h.json != nil {
		jsonRecord := record.Clone()
		if h.component != "" {
			jsonRecord.AddAttrs(slog.String("component", h.component))
		}
		if err := h.json.Handle(ctx, jsonRecord); err != nil {
			result = errors.Join(result, err)
		}
	}

	return result
}

func (h rootHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var console slog.Handler
	if h.console != nil {
		console = h.console.WithAttrs(attrs)
	}

	var json slog.Handler
	if h.json != nil {
		json = h.json.WithAttrs(attrs)
	}

	return rootHandler{
		console:   console,
		json:      json,
		component: h.component,
	}
}

func (h rootHandler) WithGroup(name string) slog.Handler {
	var console slog.Handler
	if h.console != nil {
		console = h.console.WithGroup(name)
	}

	var json slog.Handler
	if h.json != nil {
		json = h.json.WithGroup(name)
	}

	return rootHandler{
		console:   console,
		json:      json,
		component: h.component,
	}
}

func (h rootHandler) WithComponent(component string) slog.Handler {
	h.component = component
	if withComponentHandler, ok := h.console.(interface{ WithComponent(string) slog.Handler }); ok {
		h.console = withComponentHandler.WithComponent(component)
	}
	return h
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
