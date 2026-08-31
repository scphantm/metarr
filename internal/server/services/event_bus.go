package services

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// EventBusServer implements metarrv1connect.EventBusServiceHandler: it views
// and edits the event_bus config section (stream caps, retention window,
// Router retry policy). Like every other config section its write goes
// through the config store as a scoped mutation — cfg.EventBus = config —
// never a whole-document write (docs/adr/0001).
//
// The Router and the retention sweep read this section at startup, so an
// edit here takes effect on the next server restart rather than live. The
// screen says so.
type EventBusServer struct {
	*handlers.Handlers
}

// EventBusAuthPolicies is this service's method-name -> policy map, matching
// every other config section: GroupConfig, read-only for the getter.
var EventBusAuthPolicies = map[string]httpserver.RPCPolicy{
	"GetConfig":    {Group: auth.GroupConfig, ReadOnly: true},
	"UpdateConfig": {Group: auth.GroupConfig},
}

func (s *EventBusServer) GetConfig(
	ctx context.Context,
	req *connect.Request[metarrv1.EventBusServiceGetConfigRequest],
) (*connect.Response[metarrv1.EventBusServiceGetConfigResponse], error) {
	appConfig := appconfig.Get()
	return connect.NewResponse(&metarrv1.EventBusServiceGetConfigResponse{
		Config: cloneMsg(appConfig.EventBus),
	}), nil
}

func (s *EventBusServer) UpdateConfig(
	ctx context.Context,
	req *connect.Request[metarrv1.EventBusServiceUpdateConfigRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	entry := req.Msg.GetConfig()
	if entry == nil {
		return nil, connectError(http.StatusBadRequest, fmt.Errorf("event_bus config is required"))
	}
	entry = cloneMsg(entry)

	if err := validateEventBusConfig(entry); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		cfg.EventBus = entry
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

// validateEventBusConfig rejects a section that would break the bus: a
// non-positive cap drops every message, a negative retry count is
// nonsensical, and a max backoff below the base is contradictory.
func validateEventBusConfig(c *metarrv1.EventBusConfig) error {
	switch {
	case c.GetMaxLen() <= 0:
		return fmt.Errorf("max_len must be greater than zero")
	case c.GetRetentionHours() < 1:
		return fmt.Errorf("retention_hours must be at least 1")
	case c.GetRetryAttempts() < 0:
		return fmt.Errorf("retry_attempts cannot be negative")
	case c.GetRetryBackoffBaseMs() < 1:
		return fmt.Errorf("retry_backoff_base_ms must be at least 1")
	case c.GetRetryBackoffMaxMs() < c.GetRetryBackoffBaseMs():
		return fmt.Errorf("retry_backoff_max_ms must be at least retry_backoff_base_ms")
	}
	return nil
}
