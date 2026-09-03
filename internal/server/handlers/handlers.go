// Package handlers implements the HTTP handlers for the API. Each handler is
// deliberately thin: it derives a correlation ID, talks to the event bus,
// and shapes the response — all actual work happens in the listeners.
package handlers

import (
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"Metarr/internal/server/agentregistry"
	"Metarr/internal/server/appconfigstore"
	"Metarr/internal/server/busstats"
	"Metarr/internal/server/logtail"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/workflow"
)

// Handlers bundles the dependencies shared by every HTTP handler.
type Handlers struct {
	Bus                *eventbus.Bus
	AppConfigStore     *appconfigstore.Store
	LocalDirectoryRepo *mongostore.LocalDirectoryRepo
	WorkflowCatalog    *workflow.Catalog
	Stats              *busstats.Sampler
	Redis              redis.UniversalClient
	Agents             *agentregistry.Registry
	LogTail            *logtail.Buffer
	Logger             *slog.Logger
	HeartbeatTimeout   time.Duration
}

// New constructs a Handlers from its dependencies.
func New(
	bus *eventbus.Bus,
	appConfigStore *appconfigstore.Store,
	localDirectoryRepo *mongostore.LocalDirectoryRepo,
	workflowCatalog *workflow.Catalog,
	stats *busstats.Sampler,
	redisClient redis.UniversalClient,
	agents *agentregistry.Registry,
	logTail *logtail.Buffer,
	logger *slog.Logger,
	heartbeatTimeout time.Duration,
) *Handlers {
	return &Handlers{
		Bus:                bus,
		AppConfigStore:     appConfigStore,
		LocalDirectoryRepo: localDirectoryRepo,
		WorkflowCatalog:    workflowCatalog,
		Stats:              stats,
		Redis:              redisClient,
		Agents:             agents,
		LogTail:            logTail,
		Logger:             logger,
		HeartbeatTimeout:   heartbeatTimeout,
	}
}
