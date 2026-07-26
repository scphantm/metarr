package listeners

import (
	"context"
	"encoding/json"
	"log/slog"

	"Metarr/internal/appconfig"
	"Metarr/internal/eventbus"
)

// RunSystemConfigUpdateListener consumes system_config_update events. For
// each event it persists the updated document as the singleton config
// record in MongoDB, then swaps the in-memory config singleton to the new
// settings so the rest of the process sees the update immediately.
func RunSystemConfigUpdateListener(ctx context.Context, bus *eventbus.StreamBus, repo *appconfig.Repo, logger *slog.Logger) error {
	logger.Info("system_config_update listener started", "stream", eventbus.SystemConfigUpdateStream)

	return bus.Consume(ctx, eventbus.SystemConfigUpdateStream, eventbus.SystemConfigUpdateGroup, "worker-1", func(ctx context.Context, evt eventbus.Event) error {
		var cfg appconfig.Config
		if err := json.Unmarshal(evt.Payload, &cfg); err != nil {
			logger.Error("system_config_update listener: invalid payload", "correlation_id", evt.CorrelationID, "error", err)
			return err
		}

		if err := repo.Upsert(ctx, &cfg); err != nil {
			logger.Error("failed to persist system config update", "correlation_id", evt.CorrelationID, "error", err)
			return err
		}

		appconfig.Set(&cfg)
		logger.Info("system config updated", "correlation_id", evt.CorrelationID)
		return nil
	})
}
