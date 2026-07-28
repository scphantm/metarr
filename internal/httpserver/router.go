package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "Metarr/docs"
	"Metarr/internal/auth"
	"Metarr/internal/handlers"
	"Metarr/internal/session"
)

// throttledInterval caps the heartbeat and auth (login/logout) endpoints at
// one call each per this duration — each gets its own independent budget.
const throttledInterval = 500 * time.Millisecond

// NewRouter builds the application's HTTP route table, wrapped with
// correlation ID and request logging middleware. Every route requires an
// API key except the heartbeat, login, and the Swagger UI.
func NewRouter(h *handlers.Handlers, sessions *session.Store, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	protect := func(group auth.Group, handler http.HandlerFunc) http.Handler {
		return requireAPIKey(sessions, group, handler)
	}
	throttle := func(handler http.Handler) http.Handler {
		return rateLimit(throttledInterval, handler)
	}

	// Heartbeat and login are the only API endpoints callable without a key.
	mux.Handle("GET /api/heartbeat", throttle(http.HandlerFunc(h.Heartbeat)))
	mux.Handle("POST /api/auth/login", throttle(http.HandlerFunc(h.Login)))

	mux.Handle("POST /api/auth/logout", throttle(protect(auth.GroupConfig, h.Logout)))

	mux.Handle("POST /api/tasks/sonarr_cache_data", protect(auth.GroupTasks, h.SonarrCacheData))

	mux.Handle("GET /api/config", protect(auth.GroupConfig, h.GetConfig))
	mux.Handle("PUT /api/config", protect(auth.GroupConfig, h.UpdateConfig))
	mux.Handle("PUT /api/config/admin", protect(auth.GroupConfig, h.UpdateAdmin))

	mux.Handle("GET /api/config/interfaces/sonarr", protect(auth.GroupConfig, h.ListSonarrInterfaces))
	mux.Handle("POST /api/config/interfaces/sonarr", protect(auth.GroupConfig, h.CreateSonarrInterface))
	mux.Handle("GET /api/config/interfaces/sonarr/{slug}", protect(auth.GroupConfig, h.GetSonarrInterface))
	mux.Handle("PUT /api/config/interfaces/sonarr/{slug}", protect(auth.GroupConfig, h.UpdateSonarrInterface))
	mux.Handle("DELETE /api/config/interfaces/sonarr/{slug}", protect(auth.GroupConfig, h.DeleteSonarrInterface))

	// Documentation, not part of the authenticated API surface.
	mux.HandleFunc("GET /swagger/", httpSwagger.WrapHandler)

	var handler http.Handler = mux
	handler = withLogging(logger, handler)
	handler = withCorrelationID(handler)
	return handler
}
