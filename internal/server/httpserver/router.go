package httpserver

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "Metarr/api"
	"Metarr/internal/server/handlers"
)

// ThrottledInterval caps the heartbeat and auth (login/logout) endpoints at
// one call each per this duration — each gets its own independent budget.
// Exported so cmd/metarr-server can apply the exact same budget to the
// gRPC-Web AuthService's rate-limit interceptor.
const ThrottledInterval = 500 * time.Millisecond

// ConnectService is one gRPC-Web (Connect) service ready to mount: the path
// its generated NewXServiceHandler(...) returned, and the handler itself
// (already wrapped with that service's own auth interceptor). Each REST->
// Connect migration step just appends one more entry in main.go — router.go
// itself never needs to change as domains move over.
type ConnectService struct {
	Path    string
	Handler http.Handler
}

// NewRouter builds the application's HTTP route table, wrapped with
// correlation ID and request logging middleware.
//
// GET /api/heartbeat is the only REST API endpoint left — every other
// domain, including the former WebSocket streaming topics
// (stats.redis/agents.presence/logging.tail, previously served by
// wsbus.Hub at GET /api/ws, now retired), has migrated to gRPC-Web; see the
// migration plan and internal/server/services for each domain's service.
// If uiFS is provided (non-nil), the router also serves the embedded UI
// at the root path (/) with SPA fallback.
func NewRouter(h *handlers.Handlers, logger *slog.Logger, uiFS fs.FS, connectServices []ConnectService) http.Handler {
	mux := http.NewServeMux()

	for _, svc := range connectServices {
		mux.Handle(svc.Path, svc.Handler)
	}

	throttle := func(handler http.Handler) http.Handler {
		return rateLimit(ThrottledInterval, handler)
	}

	// Heartbeat is the only REST API endpoint remaining; every gRPC-Web
	// service (including login/logout) carries its own auth interceptor and
	// is mounted above via connectServices.
	mux.Handle("GET /api/heartbeat", throttle(http.HandlerFunc(h.Heartbeat)))

	// Documentation, not part of the authenticated API surface.
	mux.HandleFunc("GET /swagger/", httpSwagger.WrapHandler)

	// Serve the embedded UI (if available) at the root path with SPA fallback.
	// This is only registered if uiFS is non-nil (production builds with -tags embed_ui).
	if uiFS != nil {
		mux.Handle("GET /", newSPAHandler(uiFS))
	}

	var handler http.Handler = mux
	handler = withLogging(logger, handler)
	handler = withCorrelationID(handler)
	return handler
}
