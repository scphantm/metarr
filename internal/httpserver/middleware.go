package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"Metarr/internal/correlation"
)

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
