// Package handlers implements the HTTP handlers for the API. Each handler is
// deliberately thin: it derives a correlation ID, talks to the event bus,
// and shapes the response — all actual work happens in the listeners.
package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/agentregistry"
	"Metarr/internal/server/appconfigstore"
	"Metarr/internal/server/busstats"
	"Metarr/internal/server/logtail"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/server/session"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/workflow"
)

// OperationStore records, completes, and reads the AIP-151 long-running
// operations behind config writes (ADR-0002 / ADR-0010). Config-mutating RPCs
// call Create then return the operation; the system_config_update listener
// calls Complete once the change is persisted. *mongostore.OperationRepo is
// the production implementation; a fake stands in for it under test.
type OperationStore interface {
	Create(ctx context.Context, name string) error
	Complete(ctx context.Context, name string, code int32, message string) error
	Get(ctx context.Context, name string) (*metarrv1.Operation, error)
	List(ctx context.Context, done *bool, limit int64) ([]*metarrv1.Operation, error)
}

// Handlers bundles the dependencies shared by every HTTP handler.
type Handlers struct {
	Bus                *eventbus.Bus
	AppConfigStore     *appconfigstore.Store
	Operations         OperationStore
	LocalDirectoryRepo *mongostore.LocalDirectoryRepo
	WorkflowRepo       *mongostore.WorkflowRepo
	WorkflowCatalog    *workflow.Catalog
	Sessions           *session.Store
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
	operations OperationStore,
	localDirectoryRepo *mongostore.LocalDirectoryRepo,
	workflowRepo *mongostore.WorkflowRepo,
	workflowCatalog *workflow.Catalog,
	sessions *session.Store,
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
		Operations:         operations,
		LocalDirectoryRepo: localDirectoryRepo,
		WorkflowRepo:       workflowRepo,
		WorkflowCatalog:    workflowCatalog,
		Sessions:           sessions,
		Stats:              stats,
		Redis:              redisClient,
		Agents:             agents,
		LogTail:            logTail,
		Logger:             logger,
		HeartbeatTimeout:   heartbeatTimeout,
	}
}
