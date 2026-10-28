package logger

import (
	"context"
	"log/slog"
	"sync"
)

// asyncSink wraps synchronous handlers (console, file) to make them non-blocking.
type asyncSink struct {
	handler    slog.Handler
	ch         chan slog.Record
	closeCh    chan struct{}
	stats      *sinkStats
	sinkGrp    *sinkGroup      // Reference for shutdown coordination
	shutdownWg *sync.WaitGroup // Pointer to sinkGrp.shutdownWg
}

func newAsyncSink(handler slog.Handler, bufferSize int, sinkGrp *sinkGroup) *asyncSink {
	if bufferSize <= 0 {
		bufferSize = 1024 // Default buffer
	}

	s := &asyncSink{
		handler:    handler,
		ch:         make(chan slog.Record, bufferSize),
		closeCh:    sinkGrp.closeCh,
		stats:      &sinkStats{},
		sinkGrp:    sinkGrp,
		shutdownWg: &sinkGrp.shutdownWg,
	}

	s.shutdownWg.Add(1)
	go s.run()

	return s
}

func (s *asyncSink) run() {
	defer s.shutdownWg.Done()

	for {
		select {
		case record := <-s.ch:
			// Synchronous write to underlying handler (tint/JSON)
			if err := s.handler.Handle(context.Background(), record); err != nil {
				s.stats.sendErrors.Add(1)
			}
		case <-s.closeCh:
			// Drain remaining records on shutdown
			for {
				select {
				case record := <-s.ch:
					_ = s.handler.Handle(context.Background(), record)
				default:
					return
				}
			}
		}
	}
}

// Handle is non-blocking: enqueue or drop.
func (s *asyncSink) Handle(ctx context.Context, record slog.Record) error {
	select {
	case s.ch <- record:
		return nil
	default:
		// Buffer full — drop and count
		s.stats.droppedOverflow.Add(1)
		return nil // Non-blocking contract
	}
}

func (s *asyncSink) Enabled(ctx context.Context, level slog.Level) bool {
	return s.handler.Enabled(ctx, level)
}

func (s *asyncSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &asyncSink{
		handler:    s.handler.WithAttrs(attrs),
		ch:         s.ch,
		closeCh:    s.closeCh,
		stats:      s.stats,
		sinkGrp:    s.sinkGrp,
		shutdownWg: s.shutdownWg,
	}
}

func (s *asyncSink) WithGroup(name string) slog.Handler {
	return &asyncSink{
		handler:    s.handler.WithGroup(name),
		ch:         s.ch,
		closeCh:    s.closeCh,
		stats:      s.stats,
		sinkGrp:    s.sinkGrp,
		shutdownWg: s.shutdownWg,
	}
}
