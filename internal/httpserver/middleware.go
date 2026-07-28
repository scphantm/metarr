package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"Metarr/internal/appconfig"
	"Metarr/internal/auth"
	"Metarr/internal/correlation"
	"Metarr/internal/session"
)

const (
	apiKeyHeaderName = "X-Api-Key"
	apiKeyQueryParam = "apikey"
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

// requireAPIKey wraps next so it only runs for requests carrying a valid
// API key (via the X-Api-Key header, or an apikey query parameter as a
// fallback) whose role is authorized for group. Callers with no key, an
// unrecognized key, or a key whose role isn't authorized for group are
// rejected before next ever runs. A key currently valid in sessions (i.e.
// issued by POST /api/auth/login) always carries admin rights, checked
// before falling back to the static config-based key categories.
func requireAPIKey(sessions *session.Store, group auth.Group, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get(apiKeyHeaderName)
		if apiKey == "" {
			apiKey = r.URL.Query().Get(apiKeyQueryParam)
		}

		role := auth.RoleAdmin
		if !sessions.Valid(r.Context(), apiKey) {
			resolvedRole, ok := auth.Resolve(appconfig.Get(), apiKey)
			if !ok {
				http.Error(w, "missing or invalid API key", http.StatusUnauthorized)
				return
			}
			role = resolvedRole
		}

		if !auth.Authorized(role, group, r.Method) {
			http.Error(w, "API key not authorized for this endpoint", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r.WithContext(auth.WithAPIKey(r.Context(), apiKey)))
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
