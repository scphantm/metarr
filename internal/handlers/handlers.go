// Package handlers implements the HTTP handlers for the API. Each handler is
// deliberately thin: it derives a correlation ID, talks to the event bus,
// and shapes the response — all actual work happens in the listeners.
package handlers

import (
	"log/slog"
	"time"

	"Metarr/internal/appconfig"
	"Metarr/internal/eventbus"
	"Metarr/internal/session"
)

// Handlers bundles the dependencies shared by every HTTP handler.
type Handlers struct {
	PubSub           *eventbus.PubSubBus
	Streams          *eventbus.StreamBus
	AppConfigRepo    *appconfig.Repo
	Sessions         *session.Store
	Logger           *slog.Logger
	HeartbeatTimeout time.Duration
}

// New constructs a Handlers from its dependencies.
func New(pubsub *eventbus.PubSubBus, streams *eventbus.StreamBus, appConfigRepo *appconfig.Repo, sessions *session.Store, logger *slog.Logger, heartbeatTimeout time.Duration) *Handlers {
	return &Handlers{
		PubSub:           pubsub,
		Streams:          streams,
		AppConfigRepo:    appConfigRepo,
		Sessions:         sessions,
		Logger:           logger,
		HeartbeatTimeout: heartbeatTimeout,
	}
}

// AcceptedResponse is the shared response shape for endpoints that fire an
// event and return before it's been processed.
type AcceptedResponse struct {
	Status        string `json:"status"`
	Event         string `json:"event"`
	CorrelationID string `json:"correlation_id"`
}
