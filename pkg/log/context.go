package log

import (
	"context"

	"github.com/rs/zerolog"
)

type ctxKey struct{}

// WithLogger stores a logger in the context.
func WithLogger(ctx context.Context, logger zerolog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// Ctx retrieves the logger from the context.
// If no logger is found, the global logger is returned.
func Ctx(ctx context.Context) zerolog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(zerolog.Logger); ok {
		return l
	}
	return L()
}
