package logger

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

var (
	vlEntryPool = sync.Pool{
		New: func() any {
			m := make(map[string]any, 16)
			return &m
		},
	}
	vlStreamPool = sync.Pool{
		New: func() any {
			m := make(map[string]string, 4)
			return &m
		},
	}
)

// victoriaLogsEncoder implements recordEncoder for VictoriaLogs.
type victoriaLogsEncoder struct {
	streamFields    []string
	staticLabels    map[string]string
	streamFieldSet  map[string]bool // pre-computed set for fast lookups
}

func (e *victoriaLogsEncoder) contentType() string {
	return "application/stream+json"
}

func (e *victoriaLogsEncoder) encodeRecords(records []slog.Record) ([]byte, error) {
	var buf strings.Builder

	for _, record := range records {
		line := e.encodeRecord(record)
		buf.WriteString(line)
		buf.WriteString("\n")
	}

	return []byte(buf.String()), nil
}

// encodeRecord converts a single slog.Record to VictoriaLogs JSON line format.
func (e *victoriaLogsEncoder) encodeRecord(record slog.Record) string {
	entryPtr := vlEntryPool.Get().(*map[string]any)
	entry := *entryPtr
	// Clear map but keep capacity
	for k := range entry {
		delete(entry, k)
	}
	defer vlEntryPool.Put(entryPtr)

	// Build _msg with message + all attributes in key=value format
	var msgBuf strings.Builder
	msgBuf.WriteString(record.Message)

	// Timestamp in RFC3339Nano (readable and precise)
	entry["_time"] = record.Time.Format(time.RFC3339Nano)

	// Add level explicitly (before attrs, so it can be overridden if needed)
	entry["level"] = levelLabel(record.Level)

	// Attributes: flatten all into the entry AND add to _msg
	record.Attrs(func(attr slog.Attr) bool {
		addAttrToMap(entry, attr, "")
		// Also add to _msg in key=value format
		msgBuf.WriteString(" ")
		formatAttrForMsg(&msgBuf, attr, "")
		return true
	})

	// Set final _msg with all attributes
	entry["_msg"] = msgBuf.String()

	// Stream fields: extract into _stream or use static labels
	streamPtr := vlStreamPool.Get().(*map[string]string)
	stream := *streamPtr
	// Clear map but keep capacity
	for k := range stream {
		delete(stream, k)
	}
	defer vlStreamPool.Put(streamPtr)

	for fieldName := range e.streamFieldSet {
		if val, ok := entry[fieldName]; ok {
			stream[fieldName] = fmt.Sprintf("%v", val)
		}
	}

	// Add static labels to stream
	for k, v := range e.staticLabels {
		stream[k] = v
	}

	// Always assign stream, even if empty, for consistency
	entry["_stream"] = stream

	// Encode as JSON line
	data, err := json.Marshal(entry)
	if err != nil {
		return ""
	}
	return string(data)
}

// newVictoriaLogsSink creates and starts a VictoriaLogs sink.
func newVictoriaLogsSink(cfg *VictoriaLogsConfig, level *slog.LevelVar, sinkGrp *sinkGroup) *httpSink {
	if cfg.URL == "" {
		return nil
	}

	if len(cfg.StreamFields) == 0 {
		cfg.StreamFields = []string{"component", "level"}
	}

	// Pre-compute stream field set for fast lookups
	streamFieldSet := make(map[string]bool, len(cfg.StreamFields))
	for _, name := range cfg.StreamFields {
		streamFieldSet[name] = true
	}

	encoder := &victoriaLogsEncoder{
		streamFields:   cfg.StreamFields,
		staticLabels:   cfg.StaticLabels,
		streamFieldSet: streamFieldSet,
	}

	return newHTTPSink(cfg.URL, &cfg.Delivery, encoder, level, sinkGrp)
}
