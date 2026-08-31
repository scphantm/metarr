package listeners

import (
	"context"
	"log/slog"

	"Metarr/internal/server/agentregistry"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/logging"
)

// RegisterSystemConfigUpdateListener registers the system_config_update
// consumer on the process Router. For each event it decodes the payload and
// hands it to a configPropagator, which persists it, swaps the in-memory
// config singleton, recompiles the sidecar registry, and republishes each
// agent's own view of the configuration — see config_propagator.go for
// exactly which of those failures are fatal to the event and which are
// logged and skipped.
//
// A fatal failure here (an undecodable payload, a persist that could not
// reach Mongo) is returned, so the Router retries it and then, once the
// retries are spent, logs it at error level and acks it (dropped) rather
// than the old listener's silent redelivery loop.
//
// Agents must never subscribe to this stream: its payload is the whole
// config document, including the admin password hash, every API key and
// every Sonarr credential. They read a redacted per-agent projection
// instead, which is what the republish step writes — see
// agentregistry.BuildProjection.
func RegisterSystemConfigUpdateListener(
	router *eventbus.Router,
	repo *mongostore.AppConfigRepo,
	agents *agentregistry.Registry,
	logShipper *logging.Shipper,
	logger *slog.Logger,
) error {
	logger.Info("registering system_config_update listener", "stream", eventbus.SystemConfigUpdateStream)

	propagator := newConfigPropagator(repo, liveConfigSetterFunc(appconfig.Set), sidecarRegistryAdapter{}, agents, logShipper, logger)

	return router.Handle(
		eventbus.SystemConfigUpdateTopic(),
		eventbus.ConsumerName,
		func(ctx context.Context, event *eventbus.Event) error {
			// Decode through the same protojson path the payload was published
			// with (appconfig.MarshalStored). Reading a proto message back with
			// encoding/json is what the old listener did, and it silently
			// mishandled well-known types like the timestamps in this document.
			cfg, err := appconfig.UnmarshalStored(event.Payload)
			if err != nil {
				logger.Error("system_config_update listener: invalid payload", "correlation_id", event.CorrelationId, "error", err)
				return err
			}

			ctx = correlation.WithID(ctx, event.CorrelationId)
			if err := propagator.Apply(ctx, cfg); err != nil {
				return err
			}

			logger.Info("system config updated", "correlation_id", event.CorrelationId)
			return nil
		},
	)
}
