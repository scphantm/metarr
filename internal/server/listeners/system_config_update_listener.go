package listeners

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"

	"Metarr/internal/server/agentregistry"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/logging"
)

// operationCompleter finishes the AIP-151 operation a config write opened, once
// this listener has (or has not) persisted the change. *mongostore.OperationRepo
// satisfies it. A write from a service that has not migrated to operations yet
// has no record under the correlation id; Complete upserts, so the listener
// still leaves a consistent (if unqueried) row and never fails the event over
// a missing one.
type operationCompleter interface {
	Complete(ctx context.Context, name string, opCode int32, opMessage string) error
}

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
	bus *eventbus.Bus,
	repo *mongostore.AppConfigRepo,
	operations operationCompleter,
	agents *agentregistry.Registry,
	logShipper *logging.Shipper,
	logger *slog.Logger,
) error {
	logger.Info("registering system_config_update listener", "stream", eventbus.SystemConfigUpdateStream)

	propagator := newConfigPropagator(repo, liveConfigSetterFunc(appconfig.Set), sidecarRegistryAdapter{}, agents, logShipper, logger)

	return bus.HandleStream(
		eventbus.SystemConfigUpdateTopic(),
		map[string]eventbus.StreamHandler{
			eventbus.SystemConfigUpdateEventName: func(ctx context.Context, event *eventbus.Event) error {
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
				applyErr := propagator.Apply(ctx, cfg)

				// Finish the AIP-151 operation the write opened. On a persist
				// failure the operation is completed with the error and the
				// event is still returned for the Router to retry; a later
				// retry that succeeds flips the operation back to done-ok.
				// completeOperation only logs its own failure — the config
				// change itself has already landed (or not) regardless.
				completeOperation(ctx, operations, event.CorrelationId, applyErr, logger)

				if applyErr != nil {
					return applyErr
				}

				logger.Info("system config updated", "correlation_id", event.CorrelationId)
				return nil
			},
		},
	)
}

// completeOperation marks the operation for correlationID done. applyErr nil is
// success; otherwise its Connect code and message are recorded.
func completeOperation(ctx context.Context, operations operationCompleter, correlationID string, applyErr error, logger *slog.Logger) {
	name := "operations/" + correlationID

	var code int32
	var message string
	if applyErr != nil {
		code = int32(connect.CodeOf(applyErr))
		message = "failed to persist the configuration change"
	}

	if err := operations.Complete(ctx, name, code, message); err != nil {
		logger.Error("failed to finish config operation", "operation", name, "error", err)
	}
}
