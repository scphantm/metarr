package listeners

import (
	"context"
	"log/slog"

	"Metarr/internal/server/logtail"
	"Metarr/internal/shared/eventbus"
)

// RunLogTailListener subscribes to the centralized logging channel and feeds
// every record it sees into buffer, which backs the live tail on the
// System > Logging screen. It sees records from every process publishing to
// eventbus.LogChannel — the server itself included — not only whichever one
// happens to be running this listener.
func RunLogTailListener(ctx context.Context, bus *eventbus.PubSubBus, buffer *logtail.Buffer, logger *slog.Logger) {
	subscription := bus.Subscribe(ctx, eventbus.LogChannel)
	defer subscription.Close()

	logger.Info("log tail listener started", "channel", eventbus.LogChannel)

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-subscription.Channel():
			if !ok {
				return
			}
			buffer.Add([]byte(msg.Payload))
		}
	}
}
