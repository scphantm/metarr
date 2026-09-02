package listeners

import (
	"context"
	"log/slog"

	"Metarr/internal/server/logtail"
	"Metarr/internal/shared/eventbus"
)

// RegisterLogTailListener registers a notify handler on the Bus that feeds
// every record published on eventbus.LogChannel into buffer, which backs the
// live tail on the System > Logging screen. It sees records from every
// process publishing to the channel — the server itself included — not only
// whichever one happens to be running this listener. The Bus owns the
// subscription loop; multiple notify handlers on the one channel each get
// their own subscription, so this and RegisterLogForwardListener stay
// independent.
func RegisterLogTailListener(bus *eventbus.Bus, buffer *logtail.Buffer, logger *slog.Logger) error {
	if err := bus.HandleNotify(eventbus.LogTopic(), func(_ context.Context, payload []byte) {
		buffer.Add(payload)
	}); err != nil {
		return err
	}
	logger.Info("log tail listener registered", "channel", eventbus.LogChannel)
	return nil
}
