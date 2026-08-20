// Package handlers implements the HTTP handlers for the API. Each handler is
// deliberately thin: it derives a correlation ID, talks to the event bus,
// and shapes the response — all actual work happens in the listeners.
package handlers

import (
	"log/slog"
	"time"

	"Metarr/internal/server/agentregistry"
	"Metarr/internal/server/chatbot"
	"Metarr/internal/server/logtail"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/server/redisstats"
	"Metarr/internal/server/session"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/workflow"
)

// Handlers bundles the dependencies shared by every HTTP handler.
type Handlers struct {
	PubSub             *eventbus.PubSubBus
	Streams            *eventbus.StreamBus
	AppConfigRepo      *mongostore.AppConfigRepo
	LocalDirectoryRepo *mongostore.LocalDirectoryRepo
	WorkflowRepo       *mongostore.WorkflowRepo
	WorkflowCatalog    *workflow.Catalog
	ChatbotRepo        *mongostore.ChatbotRepo
	ChatbotService     *chatbot.Service
	Sessions           *session.Store
	Stats              *redisstats.Collector
	Agents             *agentregistry.Registry
	LogTail            *logtail.Buffer
	Logger             *slog.Logger
	HeartbeatTimeout   time.Duration
}

// New constructs a Handlers from its dependencies.
func New(
	pubsub *eventbus.PubSubBus,
	streams *eventbus.StreamBus,
	appConfigRepo *mongostore.AppConfigRepo,
	localDirectoryRepo *mongostore.LocalDirectoryRepo,
	workflowRepo *mongostore.WorkflowRepo,
	workflowCatalog *workflow.Catalog,
	chatbotRepo *mongostore.ChatbotRepo,
	chatbotService *chatbot.Service,
	sessions *session.Store,
	stats *redisstats.Collector,
	agents *agentregistry.Registry,
	logTail *logtail.Buffer,
	logger *slog.Logger,
	heartbeatTimeout time.Duration,
) *Handlers {
	return &Handlers{
		PubSub:             pubsub,
		Streams:            streams,
		AppConfigRepo:      appConfigRepo,
		LocalDirectoryRepo: localDirectoryRepo,
		WorkflowRepo:       workflowRepo,
		WorkflowCatalog:    workflowCatalog,
		ChatbotRepo:        chatbotRepo,
		ChatbotService:     chatbotService,
		Sessions:           sessions,
		Stats:              stats,
		Agents:             agents,
		LogTail:            logTail,
		Logger:             logger,
		HeartbeatTimeout:   heartbeatTimeout,
	}
}

// AcceptedResponse is the shared response shape for endpoints that fire an
// event and return before it's been processed.
type AcceptedResponse struct {
	Status        string `json:"status"`
	Event         string `json:"event"`
	CorrelationID string `json:"correlation_id"`
}
