package logger

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// httpSink manages buffered HTTP delivery of log records to a remote backend.
type httpSink struct {
	handler slog.Handler
	stats   *sinkStats

	config *DeliveryConfig
	url    string

	bufferCh  chan slog.Record
	ticker    *time.Ticker
	stopCh    chan struct{}
	sinkGroup *sinkGroup // Reference to parent sink group for shutdown
	ctx       context.Context // Context for outgoing HTTP requests

	encoder recordEncoder
	client  *http.Client
}

// recordEncoder knows how to encode a batch of records for a specific backend.
type recordEncoder interface {
	// encodeRecords transforms slog.Record slice into the backend's wire format
	encodeRecords(records []slog.Record) ([]byte, error)
	// contentType returns the Content-Type header value
	contentType() string
}

// newHTTPSink creates and starts an HTTP sink with the given encoder.
func newHTTPSink(url string, cfg *DeliveryConfig, encoder recordEncoder, level *slog.LevelVar, sinkGrp *sinkGroup) *httpSink {
	// Create context that gets cancelled when sinkGrp closes
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-sinkGrp.closeCh
		cancel()
	}()

	sink := &httpSink{
		config:   cfg,
		url:      url,
		bufferCh: make(chan slog.Record, cfg.BufferSize),
		ticker:   time.NewTicker(cfg.FlushInterval),
		stopCh:   make(chan struct{}),
		encoder:  encoder,
		ctx:      ctx,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		stats: &sinkStats{},
		handler: &levelFilterHandler{
			handler:     &httpSinkHandler{bufferCh: nil}, // filled below
			globalLevel: level,
			sinkLevel:   nil,
		},
	}

	// Inject buffer channel into the sink handler
	sink.handler.(*levelFilterHandler).handler.(*httpSinkHandler).bufferCh = sink.bufferCh
	sink.handler.(*levelFilterHandler).handler.(*httpSinkHandler).stats = sink.stats

	// Start worker goroutine
	sinkGrp.shutdownWg.Add(1)
	go sink.worker(sinkGrp.closeCh, &sinkGrp.shutdownWg)

	return sink
}

// WithAttrs returns a new httpSink with accumulated attributes.
func (s *httpSink) WithAttrs(attrs []slog.Attr) *httpSink {
	newSink := *s // shallow copy
	newSink.handler = s.handler.WithAttrs(attrs)
	return &newSink
}

// WithGroup returns a new httpSink with a group name added.
func (s *httpSink) WithGroup(name string) *httpSink {
	newSink := *s // shallow copy
	newSink.handler = s.handler.WithGroup(name)
	return &newSink
}

// httpSinkHandler is the slog.Handler that buffers records.
type httpSinkHandler struct {
	bufferCh chan slog.Record
	stats    *sinkStats
	groups   []string    // Active groups from WithGroup
	preAttrs []slog.Attr // Attributes added via WithAttrs
}

func (h *httpSinkHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *httpSinkHandler) Handle(ctx context.Context, record slog.Record) error {
	// If we have groups or preAttrs, wrap them into the record
	if len(h.groups) > 0 || len(h.preAttrs) > 0 {
		record = h.wrapRecord(record)
	}

	select {
	case h.bufferCh <- record.Clone():
		return nil
	default:
		// Buffer full: drop oldest and add new
		select {
		case <-h.bufferCh:
			h.stats.droppedOverflow.Add(1)
		default:
		}
		select {
		case h.bufferCh <- record.Clone():
		default:
			h.stats.droppedOverflow.Add(1)
		}
		return nil
	}
}

// wrapRecord creates a new record with groups and preAttrs merged.
func (h *httpSinkHandler) wrapRecord(record slog.Record) slog.Record {
	if len(h.groups) == 0 && len(h.preAttrs) == 0 {
		return record
	}

	// Collect all attributes (preAttrs first, then record attrs)
	allAttrs := make([]slog.Attr, 0, len(h.preAttrs)+record.NumAttrs())
	allAttrs = append(allAttrs, h.preAttrs...)
	record.Attrs(func(a slog.Attr) bool {
		allAttrs = append(allAttrs, a)
		return true
	})

	// Wrap in groups from innermost to outermost
	if len(h.groups) > 0 {
		for i := len(h.groups) - 1; i >= 0; i-- {
			// Convert attrs to []any for slog.Group
			values := make([]any, 0, len(allAttrs)*2)
			for _, attr := range allAttrs {
				values = append(values, attr.Key, attr.Value.Any())
			}
			allAttrs = []slog.Attr{slog.Group(h.groups[i], values...)}
		}
	}

	// Create new record with wrapped attributes
	newRecord := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	newRecord.AddAttrs(allAttrs...)
	return newRecord
}

func (h *httpSinkHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newPreAttrs := make([]slog.Attr, 0, len(h.preAttrs)+len(attrs))
	newPreAttrs = append(newPreAttrs, h.preAttrs...)
	newPreAttrs = append(newPreAttrs, attrs...)

	return &httpSinkHandler{
		bufferCh: h.bufferCh,
		stats:    h.stats,
		groups:   h.groups,
		preAttrs: newPreAttrs,
	}
}

func (h *httpSinkHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &httpSinkHandler{
		bufferCh: h.bufferCh,
		stats:    h.stats,
		groups:   newGroups,
		preAttrs: h.preAttrs,
	}
}

// worker runs the batching and delivery loop.
func (sink *httpSink) worker(closeCh chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	defer sink.ticker.Stop()

	batch := make([]slog.Record, 0, sink.config.BatchSize)

	for {
		select {
		case record, ok := <-sink.bufferCh:
			if !ok {
				return // Channel closed during shutdown
			}
			batch = append(batch, record)
			if len(batch) >= sink.config.BatchSize {
				sink.flush(batch)
				batch = batch[:0]
				sink.ticker.Reset(sink.config.FlushInterval)
			}

		case <-sink.ticker.C:
			if len(batch) > 0 {
				sink.flush(batch)
				batch = batch[:0]
			}

		case <-closeCh:
			// Drain remaining records before stopping
			close(sink.bufferCh)
			for record := range sink.bufferCh {
				batch = append(batch, record)
				if len(batch) >= sink.config.BatchSize {
					sink.flush(batch)
					batch = batch[:0]
				}
			}
			if len(batch) > 0 {
				sink.flush(batch)
			}
			return
		}
	}
}

// flush sends a batch with retries only for retryable errors.
func (sink *httpSink) flush(records []slog.Record) {
	if len(records) == 0 {
		return
	}

	payload, err := sink.encoder.encodeRecords(records)
	if err != nil {
		sink.stats.sendErrors.Add(uint64(len(records)))
		return
	}

	waitTime := sink.config.RetryInitialWait
	for attempt := 0; attempt < sink.config.MaxRetries; attempt++ {
		success, retryable := sink.send(payload)
		if success {
			return
		}

		// Don't retry if not retryable (e.g., 4xx errors)
		if !retryable {
			sink.stats.droppedRetriesExhausted.Add(uint64(len(records)))
			return
		}

		if attempt < sink.config.MaxRetries-1 {
			// Add jitter to avoid thundering herd
			jitter := time.Duration(rand.Int63n(int64(waitTime)))
			time.Sleep(waitTime + jitter)

			// Exponential backoff with cap
			waitTime = time.Duration(math.Min(
				float64(waitTime*2),
				float64(sink.config.RetryMaxWait),
			))
		}
	}

	// All retries exhausted
	sink.stats.droppedRetriesExhausted.Add(uint64(len(records)))
}

// send attempts a single HTTP POST and returns (success, retryable).
func (sink *httpSink) send(payload []byte) (bool, bool) {
	req, err := http.NewRequestWithContext(sink.ctx, http.MethodPost, sink.url, bytes.NewReader(payload))
	if err != nil {
		sink.stats.sendErrors.Add(1)
		return false, true // Network error, retryable
	}

	req.Header.Set("Content-Type", sink.encoder.contentType())

	resp, err := sink.client.Do(req)
	if err != nil {
		sink.stats.sendErrors.Add(1)
		return false, true // Network error, retryable
	}
	defer resp.Body.Close()

	// Retry on 5xx, 429, 408
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout {
		return false, true // Retryable server/transient error
	}

	// Success on 2xx
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, false // Success, no retry needed
	}

	// Don't retry on other 4xx (bad request, not found, etc.)
	sink.stats.sendErrors.Add(1)
	return false, false // Client error, not retryable
}

// Satisfy slog.Handler interface for rootHandler.
func (sink *httpSink) Enabled(ctx context.Context, level slog.Level) bool {
	return sink.handler.Enabled(ctx, level)
}

func (sink *httpSink) Handle(ctx context.Context, record slog.Record) error {
	return sink.handler.Handle(ctx, record)
}
