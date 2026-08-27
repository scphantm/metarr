package handlers

import (
	"context"
	"encoding/json"
	"time"

	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/eventbus"
)

// GetConfig and UpdateConfig migrated to gRPC-Web — see
// metarr.v1.ConfigService (internal/server/services/config.go), mounted via
// connectServices in cmd/metarr-server/main.go.

// FireConfigUpdate marshals config and fires it as a system_config_update
// event. Used for both whole-document updates (UpdateConfig) and
// per-interface-instance CRUD, which read-modify-fire the same full
// document so the existing SystemConfigUpdate listener can persist it and
// refresh the in-memory config singleton without any changes of its own.
// Exported so the gRPC-Web services in internal/server/services (which
// embed *Handlers) can call it too during the REST->Connect migration.
func (h *Handlers) FireConfigUpdate(ctx context.Context, correlationID string, config appconfig.Config) error {
	payload, err := json.Marshal(config)
	if err != nil {
		return err
	}

	event := eventbus.Event{
		CorrelationID: correlationID,
		Name:          eventbus.SystemConfigUpdateEventName,
		Payload:       payload,
		Timestamp:     time.Now().UTC(),
	}

	return h.Streams.Fire(ctx, eventbus.SystemConfigUpdateStream, event)
}
