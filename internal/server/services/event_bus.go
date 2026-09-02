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
	"GetEventBusConfig":    {Group: auth.GroupConfig, ReadOnly: true},
	"UpdateEventBusConfig": {Group: auth.GroupConfig},
}

func (s *EventBusServer) GetEventBusConfig(
	ctx context.Context,
	req *connect.Request[metarrv1.GetEventBusConfigRequest],
) (*connect.Response[metarrv1.GetEventBusConfigResponse], error) {
	return connect.NewResponse(&metarrv1.GetEventBusConfigResponse{
		Config: cloneMsg(appconfig.Get().EventBus),
	}), nil
}

// UpdateEventBusConfig is an AIP-134 partial update: update_mask names the
// EventBusConfig fields to change, req.Config carries their new values, and the
// masked fields are merged onto the stored section under the config store's
// lock. An empty mask or an unknown path returns InvalidArgument; the merged
// section is then validated as a whole so a partial edit can't leave a
// contradictory combination (a max backoff below the base). The write is
// synchronous — it persists and propagates in-process before returning the
// stored section (docs/adr/0002).
func (s *EventBusServer) UpdateEventBusConfig(
	ctx context.Context,
	req *connect.Request[metarrv1.UpdateEventBusConfigRequest],
) (*connect.Response[metarrv1.EventBusConfig], error) {
	correlationID := correlation.FromContext(ctx)

	patch := req.Msg.GetConfig()
	if patch == nil {
		return nil, connectError(http.StatusBadRequest, fmt.Errorf("event_bus config is required"))
	}

	var stored *metarrv1.EventBusConfig
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		merged := cloneMsg(cfg.EventBus)
		if merged == nil {
			merged = &metarrv1.EventBusConfig{}
		}
		if err := applyUpdateMask(merged, patch, req.Msg.GetUpdateMask()); err != nil {
			return err
		}
		if err := validateEventBusConfig(merged); err != nil {
			return connectError(http.StatusBadRequest, err)
		}
		cfg.EventBus = merged
		stored = cloneMsg(merged)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}

	return connect.NewResponse(stored), nil
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
