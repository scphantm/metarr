package listeners

import (
	"context"
	"encoding/json"
	"log/slog"

	"Metarr/internal/server/agentregistry"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/logging"
)

// RunSystemConfigUpdateListener consumes system_config_update events. For
// each event it decodes the payload and hands it to a configPropagator,
// which persists it, swaps the in-memory config singleton, recompiles the
// sidecar registry, and republishes each agent's own view of the
// configuration — see config_propagator.go for exactly which of those
// failures are fatal to the event and which are logged and skipped.
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
	logShipper *logging.Shipper,
	logger *slog.Logger,
) error {
	logger.Info("system_config_update listener started", "stream", eventbus.SystemConfigUpdateStream)

	propagator := newConfigPropagator(repo, liveConfigSetterFunc(appconfig.Set), sidecarRegistryAdapter{}, agents, logShipper, logger)

	return bus.Consume(ctx, eventbus.SystemConfigUpdateStream, eventbus.SystemConfigUpdateGroup, eventbus.ConsumerName, func(ctx context.Context, event eventbus.Event) error {
		var cfg appconfig.Config
		if err := json.Unmarshal(event.Payload, &cfg); err != nil {
			logger.Error("system_config_update listener: invalid payload", "correlation_id", event.CorrelationID, "error", err)
			return err
		}

		ctx = correlation.WithID(ctx, event.CorrelationID)
		if err := propagator.Apply(ctx, &cfg); err != nil {
			return err
		}

		logger.Info("system config updated", "correlation_id", event.CorrelationID)
		return nil
	})
}
