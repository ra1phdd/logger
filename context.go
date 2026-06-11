package logger

import "context"

type contextKey struct{}

func NewContext(ctx context.Context, l *Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if l == nil {
		l = Default()
	}
	return context.WithValue(ctx, contextKey{}, l)
}

func FromContext(ctx context.Context) *Logger {
	if ctx != nil {
		if l, ok := ctx.Value(contextKey{}).(*Logger); ok && l != nil {
			return l
		}
	}
	return Default()
}
