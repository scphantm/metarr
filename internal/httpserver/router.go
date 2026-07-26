package httpserver

import (
	"log/slog"
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "Metarr/docs"
	"Metarr/internal/handlers"
)

// NewRouter builds the application's HTTP route table, wrapped with
// correlation ID and request logging middleware.
func NewRouter(h *handlers.Handlers, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/heartbeat", h.Heartbeat)
	mux.HandleFunc("POST /api/tasks/sonarr_cache_data", h.SonarrCacheData)
	mux.HandleFunc("GET /api/config", h.GetConfig)
	mux.HandleFunc("PUT /api/config", h.UpdateConfig)

	mux.HandleFunc("GET /swagger/", httpSwagger.WrapHandler)

	var handler http.Handler = mux
	handler = withLogging(logger, handler)
	handler = withCorrelationID(handler)
	return handler
}
