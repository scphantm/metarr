// Package correlation carries a correlation ID through a request's context so
// every log line and event published while handling that request can be tied
// back to it.
package correlation

import (
	"context"

	"github.com/google/uuid"
)

type contextKey struct{}

const HeaderName = "X-Correlation-ID"

// New generates a fresh correlation ID.
func New() string {
	return uuid.NewString()
}

// WithID returns a new context carrying the given correlation ID.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the correlation ID stored in ctx, or "" if none is set.
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}
