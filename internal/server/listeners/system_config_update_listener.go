package listeners

import (
	"context"
	"encoding/json"
	"log/slog"

	"Metarr/internal/server/agentregistry"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/scanmodel"
)

// RunSystemConfigUpdateListener consumes system_config_update events. For
// each event it persists the updated document as the singleton config
// record in MongoDB, then swaps the in-memory config singleton to the new
// settings so the rest of the process sees the update immediately, and
// finally republishes each agent's own view of the configuration.
//
// Agents must never subscribe to this stream: its payload is the whole config
// document, including the admin password hash, every API key and every Sonarr
// credential. They read a redacted per-agent projection instead, which is what
// the republish step writes — see agentregistry.BuildProjection.
func RunSystemConfigUpdateListener(
	ctx context.Context,
	bus *eventbus.StreamBus,
	repo *mongostore.AppConfigRepo,
	agents *agentregistry.Registry,
	logger *slog.Logger,
) error {
	logger.Info("system_config_update listener started", "stream", eventbus.SystemConfigUpdateStream)

	return bus.Consume(ctx, eventbus.SystemConfigUpdateStream, eventbus.SystemConfigUpdateGroup, "worker-1", func(ctx context.Context, event eventbus.Event) error {
		var appConfig appconfig.Config
		if err := json.Unmarshal(event.Payload, &appConfig); err != nil {
			logger.Error("system_config_update listener: invalid payload", "correlation_id", event.CorrelationID, "error", err)
			return err
		}

		if err := repo.Upsert(ctx, &appConfig); err != nil {
			logger.Error("failed to persist system config update", "correlation_id", event.CorrelationID, "error", err)
			return err
		}

		appconfig.Set(&appConfig)

		// Recompile the sidecar classification table so a change takes effect
		// on the next scan without a restart. A table that fails to compile
		// leaves the previous registry in place: the scanner must never be
		// left without one, and the previous table is a far better answer than
		// nothing.
		if registry, err := scanmodel.NewSidecarRegistry(appConfig.DirectoryScanner.SidecarTypes); err != nil {
			logger.Error("updated sidecar type table is invalid; keeping the previous one",
				"correlation_id", event.CorrelationID, "error", err)
		} else {
			scanmodel.SetSidecarRegistry(registry)
		}

		// Republish each agent's projection so a changed mapping reaches the
		// machine it applies to. A failure here is logged rather than returned:
		// the configuration is already stored and live on the server, and
		// redelivering the whole event to retry a Redis write would re-run the
		// database write too. Agents re-read on their own timer regardless.
		if err := agents.PublishAll(ctx, &appConfig); err != nil {
			logger.Error("failed to republish agent configuration",
				"correlation_id", event.CorrelationID, "error", err)
		}

		logger.Info("system config updated", "correlation_id", event.CorrelationID)
		return nil
	})
}
