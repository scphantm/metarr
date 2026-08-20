package listeners

import (
	"context"
	"log/slog"

	"Metarr/internal/server/logforward"
	"Metarr/internal/shared/eventbus"
)

// RunLogForwardListener subscribes to the centralized logging channel and
// hands every record it sees to forwarder, which relays it on to Fluent Bit.
// It runs alongside RunLogTailListener as an independent subscriber — Redis
// Pub/Sub delivers each published message to every subscriber, so the two
// listeners see identical traffic without coordinating.
func RunLogForwardListener(ctx context.Context, bus *eventbus.PubSubBus, forwarder *logforward.Forwarder, logger *slog.Logger) {
	subscription := bus.Subscribe(ctx, eventbus.LogChannel)
	defer func() { _ = subscription.Close() }()

	logger.Info("log forward listener started", "channel", eventbus.LogChannel)

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-subscription.Channel():
			if !ok {
				return
			}
			forwarder.Forward([]byte(msg.Payload))
		}
	}
}
