package logger

import (
	"log/slog"
	"time"
)

const (
	levelTrace = slog.Level(-8)
	levelFatal = slog.Level(12)

	defaultTimeFormat     = "2006-01-02 15:04:05.000000"
	defaultLogDir         = "logs"
	defaultLogFileExt     = ".log"
	defaultLogFilePattern = "2006-01-02 15-04-05.000000"

	ansiReset  = "\x1b[0m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\033[96m"
	ansiWhite  = "\x1b[37m"

	// HTTP sink defaults.
	defaultBufferSize      = 1024
	defaultBatchSize       = 512
	defaultFlushInterval   = 2 * time.Second
	defaultMaxRetries      = 3
	defaultShutdownTimeout = 3 * time.Second
	defaultRetryInitial    = 100 * time.Millisecond
	defaultRetryMax        = 10 * time.Second
)
