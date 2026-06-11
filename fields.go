package logger

import (
	"log/slog"
)

type Attr = slog.Attr

var (
	Any      = slog.Any
	String   = slog.String
	Int      = slog.Int
	Int64    = slog.Int64
	Float64  = slog.Float64
	Bool     = slog.Bool
	Duration = slog.Duration
	Time     = slog.Time
)

func Err(err error) Attr {
	if err == nil {
		return Attr{}
	}
	return slog.String("error", err.Error())
}
