package logger

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var (
	lokiKeysPool = sync.Pool{
		New: func() any {
			s := make([]string, 0, 8)
			return &s
		},
	}
	lokiEntryPool = sync.Pool{
		New: func() any {
			m := make(map[string]any, 16)
			return &m
		},
	}
	lokiLabelsPool = sync.Pool{
		New: func() any {
			m := make(map[string]string, 4)
			return &m
		},
	}
)

// lokiEncoder implements recordEncoder for Loki.
type lokiEncoder struct {
	labels       []string
	staticLabels map[string]string
	labelSet     map[string]bool // pre-computed set of label names
}

func (e *lokiEncoder) contentType() string {
	return "application/json"
}

func (e *lokiEncoder) encodeRecords(records []slog.Record) ([]byte, error) {
	// Group records by their label set
	streamGroups := make(map[string][]*lokiLogEntry)

	for _, record := range records {
		labels, logLine := e.extractLabelsAndLogLine(record)
		if labels == nil {
			continue // Skip on encoding error
		}
		key := e.labelsKey(labels)

		// Return labels to pool after using in labelsKey
		labelsPtr := &labels
		lokiLabelsPool.Put(labelsPtr)

		entry := &lokiLogEntry{
			Timestamp: record.Time.UnixNano(),
			Line:      logLine,
		}

		streamGroups[key] = append(streamGroups[key], entry)
	}

	// Build push request
	pushReq := &lokiPushRequest{
		Streams: make([]lokiStream, 0, len(streamGroups)),
	}

	for key, entries := range streamGroups {
		labels := e.keyToLabels(key)
		stream := lokiStream{
			Stream: labels,
			Values: make([][]any, len(entries)),
		}
		for i, entry := range entries {
			stream.Values[i] = []any{
				strconv.FormatInt(entry.Timestamp, 10),
				entry.Line,
			}
		}
		pushReq.Streams = append(pushReq.Streams, stream)
	}

	data, err := json.Marshal(pushReq)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// extractLabelsAndLogLine extracts labels and creates log line in a single pass.
func (e *lokiEncoder) extractLabelsAndLogLine(record slog.Record) (map[string]string, string) {
	labelsPtr := lokiLabelsPool.Get().(*map[string]string)
	labels := *labelsPtr
	// Clear map but keep capacity
	for k := range labels {
		delete(labels, k)
	}
	// Don't defer return here — labels are stored in streamGroups

	entryPtr := lokiEntryPool.Get().(*map[string]any)
	entry := *entryPtr
	// Clear map but keep capacity
	for k := range entry {
		delete(entry, k)
	}
	defer lokiEntryPool.Put(entryPtr)

	// Set base fields
	entry["msg"] = record.Message
	entry["level"] = levelLabel(record.Level)

	if e.labelSet["level"] {
		labels["level"] = levelLabel(record.Level)
	}
	if e.labelSet["component"] {
		labels["component"] = "" // Default, may be overridden
	}

	// Single pass through attributes: extract labels and add to entry
	record.Attrs(func(attr slog.Attr) bool {
		addAttrToMap(entry, attr, "")
		if e.labelSet[attr.Key] {
			attr.Value = attr.Value.Resolve()
			labels[attr.Key] = fmt.Sprintf("%v", attr.Value.Any())
		}
		return true
	})

	// Add static labels (override extracted ones)
	for k, v := range e.staticLabels {
		labels[k] = v
	}

	// Encode log line
	data, err := json.Marshal(entry)
	if err != nil {
		// Return labels to pool on error
		lokiLabelsPool.Put(labelsPtr)
		return nil, ""
	}
	return labels, string(data)
}

// labelsKey creates a stable string key from a label map for grouping.
func (e *lokiEncoder) labelsKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	// Sort keys for stable ordering
	keysPtr := lokiKeysPool.Get().(*[]string)
	keys := (*keysPtr)[:0] // Reset length but keep capacity
	defer lokiKeysPool.Put(keysPtr)

	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Pre-allocate approximate size
	var buf strings.Builder
	buf.Grow(len(labels) * 16) // rough estimate: 16 chars per label

	for i, k := range keys {
		if i > 0 {
			buf.WriteString("|")
		}
		buf.WriteString(k)
		buf.WriteString("=")
		buf.WriteString(labels[k])
	}
	return buf.String()
}

// keyToLabels reconstructs label map from labelsKey.
func (e *lokiEncoder) keyToLabels(key string) map[string]string {
	labels := make(map[string]string)
	parts := strings.Split(key, "|")
	for _, part := range parts {
		if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
			labels[kv[0]] = kv[1]
		}
	}
	return labels
}

type lokiLogEntry struct {
	Timestamp int64
	Line      string
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]any           `json:"values"`
}

type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

// newLokiSink creates and starts a Loki sink.
func newLokiSink(cfg *LokiConfig, level *slog.LevelVar, sinkGrp *sinkGroup) *httpSink {
	if cfg.URL == "" {
		return nil
	}

	if len(cfg.Labels) == 0 {
		cfg.Labels = []string{"component", "level"}
	}

	// Pre-compute label set for fast lookups
	labelSet := make(map[string]bool, len(cfg.Labels))
	for _, name := range cfg.Labels {
		labelSet[name] = true
	}

	encoder := &lokiEncoder{
		labels:       cfg.Labels,
		staticLabels: cfg.StaticLabels,
		labelSet:     labelSet,
	}

	return newHTTPSink(cfg.URL, &cfg.Delivery, encoder, level, sinkGrp)
}
