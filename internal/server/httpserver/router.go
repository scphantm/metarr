package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "Metarr/docs"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/session"
	"Metarr/internal/server/wsbus"
)

// throttledInterval caps the heartbeat and auth (login/logout) endpoints at
// one call each per this duration — each gets its own independent budget.
const throttledInterval = 500 * time.Millisecond

// NewRouter builds the application's HTTP route table, wrapped with
// correlation ID and request logging middleware. Every route requires an
// API key except the heartbeat, login, and the Swagger UI.
func NewRouter(h *handlers.Handlers, hub *wsbus.Hub, sessions *session.Store, logger *slog.Logger) http.Handler {
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
	mux.Handle("POST /api/tasks/directory-scan/{slug}", protect(auth.GroupTasks, h.DirectoryScan))

	// Statistics. The REST form is what a dashboard paints before its socket
	// is up; the streaming form is the same data over the topic below.
	mux.Handle("GET /api/stats/redis", protect(auth.GroupConfig, h.GetRedisStats))

	// The streaming layer. One connection carries every topic a client asks
	// for, so this is gated on the least restrictive group and each topic
	// re-checks the caller's role against its own requirement at subscribe
	// time — see wsbus.Hub.Register.
	mux.Handle("GET /api/ws", protect(auth.GroupTasks, hub.ServeHTTP))

	mux.Handle("GET /api/config", protect(auth.GroupConfig, h.GetConfig))
	mux.Handle("PUT /api/config", protect(auth.GroupConfig, h.UpdateConfig))
	mux.Handle("PUT /api/config/admin", protect(auth.GroupConfig, h.UpdateAdmin))

	mux.Handle("GET /api/config/interfaces/sonarr", protect(auth.GroupConfig, h.ListSonarrInterfaces))
	mux.Handle("POST /api/config/interfaces/sonarr", protect(auth.GroupConfig, h.UpsertSonarrInterface))
	mux.Handle("GET /api/config/interfaces/sonarr/{slug}", protect(auth.GroupConfig, h.GetSonarrInterface))
	mux.Handle("DELETE /api/config/interfaces/sonarr/{slug}", protect(auth.GroupConfig, h.DeleteSonarrInterface))

	// Agents. An agent announces itself by connecting to Redis; these routes
	// are where someone says what it is allowed to see.
	mux.Handle("GET /api/config/agents", protect(auth.GroupConfig, h.ListAgents))
	mux.Handle("POST /api/config/agents", protect(auth.GroupConfig, h.UpsertAgent))
	mux.Handle("DELETE /api/config/agents/{slug}", protect(auth.GroupConfig, h.DeleteAgent))
	mux.Handle("POST /api/config/agents/{slug}/log-level", protect(auth.GroupConfig, h.SetAgentLogLevel))

	// Logging: the server's own level, plus the informational fields the
	// System > Logging screen shows about the Fluent Bit -> OpenObserve
	// pipeline. See appconfig.LoggingConfig for what these fields do and
	// don't control.
	mux.Handle("GET /api/config/logging", protect(auth.GroupConfig, h.GetLoggingConfig))
	mux.Handle("POST /api/config/logging", protect(auth.GroupConfig, h.UpsertLoggingConfig))

	// The live log tail on the Logging screen. GET is the first-paint
	// fallback; logging.tail is the same buffer streamed over the socket.
	mux.Handle("GET /api/logging/tail", protect(auth.GroupConfig, h.GetLogTail))

	mux.Handle("GET /api/config/directory-scanner", protect(auth.GroupConfig, h.GetDirectoryScannerConfig))
	mux.Handle("PUT /api/config/directory-scanner", protect(auth.GroupConfig, h.UpdateDirectoryScannerConfig))

	mux.Handle("GET /api/config/directory-scanner/directories", protect(auth.GroupConfig, h.ListScanDirectories))
	mux.Handle("POST /api/config/directory-scanner/directories", protect(auth.GroupConfig, h.UpsertScanDirectory))
	mux.Handle("GET /api/config/directory-scanner/directories/{slug}", protect(auth.GroupConfig, h.GetScanDirectory))
	mux.Handle("DELETE /api/config/directory-scanner/directories/{slug}", protect(auth.GroupConfig, h.DeleteScanDirectory))

	// The sidecar classification table: the rules deciding what a non-media
	// file found next to a movie or episode is.
	mux.Handle("GET /api/config/directory-scanner/sidecar-types", protect(auth.GroupConfig, h.ListSidecarTypes))
	mux.Handle("POST /api/config/directory-scanner/sidecar-types", protect(auth.GroupConfig, h.UpsertSidecarType))
	// Evaluation order is its own transaction: it covers the whole table at once
	// and is the only place an entry can be enabled or disabled.
	mux.Handle("POST /api/config/directory-scanner/sidecar-types/order", protect(auth.GroupConfig, h.ReorderSidecarTypes))
	mux.Handle("POST /api/config/directory-scanner/sidecar-types/reset", protect(auth.GroupConfig, h.ResetSidecarTypes))
	mux.Handle("GET /api/config/directory-scanner/sidecar-types/{id}", protect(auth.GroupConfig, h.GetSidecarType))
	mux.Handle("DELETE /api/config/directory-scanner/sidecar-types/{id}", protect(auth.GroupConfig, h.DeleteSidecarType))

	// Scan results. These are data reads rather than configuration, so they sit
	// in the tasks group alongside the scan trigger that produces them, which
	// also lets the read-only role query the library.
	mux.Handle("GET /api/local-directories", protect(auth.GroupTasks, h.ListLocalDirectories))
	mux.Handle("GET /api/local-directories/{id}", protect(auth.GroupTasks, h.GetLocalDirectory))
	mux.Handle("GET /api/local-directories/{id}/media-files", protect(auth.GroupTasks, h.ListDirectoryMediaFiles))
	mux.Handle("GET /api/local-directories/{id}/nfo", protect(auth.GroupTasks, h.GetLocalDirectoryNFO))
	mux.Handle("GET /api/media-files/{id}", protect(auth.GroupTasks, h.GetMediaFile))

	// Documentation, not part of the authenticated API surface.
	mux.HandleFunc("GET /swagger/", httpSwagger.WrapHandler)

	var handler http.Handler = mux
	handler = withLogging(logger, handler)
	handler = withCorrelationID(handler)
	return handler
}
