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

	bufferCh chan slog.Record
	ticker   *time.Ticker
	stopCh   chan struct{}
	wg       sync.WaitGroup
	ctx      context.Context // Context for outgoing HTTP requests

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
	go sink.worker(sinkGrp.closeCh)

	return sink
}

// httpSinkHandler is the slog.Handler that buffers records.
type httpSinkHandler struct {
	bufferCh chan slog.Record
	stats    *sinkStats
}

func (h *httpSinkHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *httpSinkHandler) Handle(ctx context.Context, record slog.Record) error {
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

func (h *httpSinkHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *httpSinkHandler) WithGroup(name string) slog.Handler {
	return h
}

// worker runs the batching and delivery loop.
func (sink *httpSink) worker(closeCh chan struct{}) {
	defer sink.wg.Done()
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

func (sink *httpSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	return sink.handler.WithAttrs(attrs)
}

func (sink *httpSink) WithGroup(name string) slog.Handler {
	return sink.handler.WithGroup(name)
}
