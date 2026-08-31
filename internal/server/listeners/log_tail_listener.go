package listeners

import (
	"context"
	"log/slog"

	"Metarr/internal/server/logtail"
	"Metarr/internal/shared/eventbus"
)

// RegisterLogTailListener registers a handler on router that feeds every
// record published on eventbus.LogChannel into buffer, which backs the live
// tail on the System > Logging screen. It sees records from every process
// publishing to the channel — the server itself included — not only whichever
// one happens to be running this listener. The router owns the subscription
// loop; multiple handlers on the one channel each get their own subscription,
// so this and RegisterLogForwardListener stay independent.
func RegisterLogTailListener(router *eventbus.PubSubRouter, buffer *logtail.Buffer, logger *slog.Logger) {
	router.Handle(eventbus.LogChannel, func(_ context.Context, payload []byte) {
		buffer.Add(payload)
	})
	logger.Info("log tail listener registered", "channel", eventbus.LogChannel)
}
