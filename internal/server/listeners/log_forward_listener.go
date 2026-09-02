package listeners

import (
	"context"
	"log/slog"

	"Metarr/internal/server/logforward"
	"Metarr/internal/shared/eventbus"
)

// RegisterLogForwardListener registers a notify handler on the Bus that hands
// every record published on eventbus.LogChannel to forwarder, which relays it
// on to Fluent Bit. It runs alongside RegisterLogTailListener as an
// independent subscriber — the Bus opens one Redis subscription per notify
// handler, and Redis Pub/Sub delivers each message to every subscriber, so
// the two log consumers see identical traffic without coordinating.
func RegisterLogForwardListener(bus *eventbus.Bus, forwarder *logforward.Forwarder, logger *slog.Logger) error {
	if err := bus.HandleNotify(eventbus.LogTopic(), func(_ context.Context, payload []byte) {
		forwarder.Forward(payload)
	}); err != nil {
		return err
	}
	logger.Info("log forward listener registered", "channel", eventbus.LogChannel)
	return nil
}
