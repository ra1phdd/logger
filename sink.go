package logger

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
)

// sink represents a single logging destination with its own handler and statistics.
type sink struct {
	name    string
	handler slog.Handler
	stats   *sinkStats
}

// sinkStats tracks operational metrics for a sink.
type sinkStats struct {
	droppedOverflow         atomic.Uint64
	droppedRetriesExhausted atomic.Uint64
	sendErrors              atomic.Uint64
}

// Stats returns a snapshot of sink statistics.
func (s *sinkStats) Stats() SinkStats {
	return SinkStats{
		DroppedOverflow:         s.droppedOverflow.Load(),
		DroppedRetriesExhausted: s.droppedRetriesExhausted.Load(),
		SendErrors:              s.sendErrors.Load(),
	}
}

// SinkStats is the public snapshot of sink operational metrics.
type SinkStats struct {
	DroppedOverflow         uint64
	DroppedRetriesExhausted uint64
	SendErrors              uint64
}

// levelFilterHandler wraps a handler with per-sink level filtering.
type levelFilterHandler struct {
	handler     slog.Handler
	globalLevel *slog.LevelVar
	sinkLevel   *slog.LevelVar
}

// Enabled checks both global and sink-specific level floors.
func (h *levelFilterHandler) Enabled(ctx context.Context, level slog.Level) bool {
	// Global floor is always enforced
	if level < h.globalLevel.Level() {
		return false
	}

	// Sink floor is an additional constraint
	if h.sinkLevel != nil && level < h.sinkLevel.Level() {
		return false
	}

	return h.handler.Enabled(ctx, level)
}

func (h *levelFilterHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.handler.Handle(ctx, r)
}

func (h *levelFilterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelFilterHandler{
		handler:     h.handler.WithAttrs(attrs),
		globalLevel: h.globalLevel,
		sinkLevel:   h.sinkLevel,
	}
}

func (h *levelFilterHandler) WithGroup(name string) slog.Handler {
	return &levelFilterHandler{
		handler:     h.handler.WithGroup(name),
		globalLevel: h.globalLevel,
		sinkLevel:   h.sinkLevel,
	}
}

// sinkGroup manages lifecycle and shutdown of multiple sinks.
type sinkGroup struct {
	sinks       []sink
	closeOnce   sync.Once
	closeCh     chan struct{}
	shutdownWg  sync.WaitGroup
	shutdownErr error
}

func newSinkGroup() *sinkGroup {
	return &sinkGroup{
		closeCh: make(chan struct{}),
	}
}

func (sg *sinkGroup) stats() []SinkStats {
	result := make([]SinkStats, len(sg.sinks))
	for i, s := range sg.sinks {
		if s.stats != nil {
			result[i] = s.stats.Stats()
		}
	}
	return result
}

func (g *sinkGroup) add(s sink) {
	g.sinks = append(g.sinks, s)
}

func (g *sinkGroup) close(ctx context.Context) error {
	g.closeOnce.Do(func() {
		close(g.closeCh)
		done := make(chan struct{})
		go func() {
			g.shutdownWg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
			g.shutdownErr = ctx.Err()
		}
	})
	return g.shutdownErr
}

// formatAttrForMsg formats a single attribute into key=value format for msg field.
// Used by both VictoriaLogs and Loki encoders.
func formatAttrForMsg(buf *strings.Builder, attr slog.Attr, prefix string) {
	key := attr.Key
	if prefix != "" {
		key = prefix + "." + key
	}

	attr.Value = attr.Value.Resolve()
	switch attr.Value.Kind() {
	case slog.KindGroup:
		// Flatten groups: group.key=value
		for _, groupAttr := range attr.Value.Group() {
			buf.WriteString(" ")
			formatAttrForMsg(buf, groupAttr, key)
		}
	default:
		buf.WriteString(key)
		buf.WriteString("=")
		buf.WriteString(fmt.Sprintf("%v", attr.Value.Any()))
	}
}
