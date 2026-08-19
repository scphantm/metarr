package listeners

import (
	"context"
	"log/slog"

	"Metarr/internal/server/mongostore"
	"Metarr/internal/shared/eventbus"
)

// RunSonarrCacheDataListener consumes sonarr_cache_data events off the
// durable event stream. For each event it logs "event fired" (to the log
// file, tagged with the event's correlation ID) and records the run in
// MongoDB, the primary data store.
func RunSonarrCacheDataListener(ctx context.Context, bus *eventbus.StreamBus, repo *mongostore.TaskEventRepo, logger *slog.Logger) error {
	logger.Info("sonarr_cache_data listener started", "stream", eventbus.SonarrCacheDataStream)

	return bus.Consume(ctx, eventbus.SonarrCacheDataStream, eventbus.SonarrCacheDataGroup, "worker-1", func(ctx context.Context, event eventbus.Event) error {
		logger.Info("event fired", "event", event.Name, "correlation_id", event.CorrelationID)

		return repo.Record(ctx, mongostore.TaskEventRecord{
			CorrelationID: event.CorrelationID,
			EventName:     event.Name,
			FiredAt:       event.Timestamp,
		})
	})
}
