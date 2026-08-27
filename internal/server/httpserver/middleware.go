package httpserver

import (
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"Metarr/internal/shared/correlation"
)

// apiKeyHeaderName is also the header connectAuthInterceptor reads — every
// gRPC-Web call and the REST-era key both carry it the same way.
const apiKeyHeaderName = "X-Api-Key"

// withCorrelationID ensures every request has a correlation ID — reusing one
// supplied by the caller via the X-Correlation-ID header, or minting a new
// one — and attaches it to the request context and response headers so it
// can be traced through every downstream event.
func withCorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := r.Header.Get(correlation.HeaderName)
		if correlationID == "" {
			correlationID = correlation.New()
		}

		w.Header().Set(correlation.HeaderName, correlationID)
		ctx := correlation.WithID(r.Context(), correlationID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// rateLimit wraps next so it only allows one call through per interval,
// globally across every caller (this is not a per-client/per-key limit —
// each call to rateLimit gets its own independent counter, so wrapping two
// different routes each gets its own separate budget). A call arriving
// less than interval after the last one that was let through is rejected
// with 429 Too Many Requests without ever reaching next.
func rateLimit(interval time.Duration, next http.Handler) http.Handler {
	var lastNano atomic.Int64

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UnixNano()
		last := lastNano.Load()
		if now-last < int64(interval) || !lastNano.CompareAndSwap(last, now) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withLogging logs each request's method, path, correlation ID, and
// duration once it completes.
func withLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"correlation_id", correlation.FromContext(r.Context()),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
