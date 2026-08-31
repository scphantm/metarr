package listeners

import (
	"context"
	"log/slog"

	"Metarr/internal/server/logforward"
	"Metarr/internal/shared/eventbus"
)

// RegisterLogForwardListener registers a handler on router that hands every
// record published on eventbus.LogChannel to forwarder, which relays it on to
// Fluent Bit. It runs alongside RegisterLogTailListener as an independent
// subscriber — the router opens one Redis subscription per handler, and Redis
// Pub/Sub delivers each message to every subscriber, so the two log consumers
// see identical traffic without coordinating.
func RegisterLogForwardListener(router *eventbus.PubSubRouter, forwarder *logforward.Forwarder, logger *slog.Logger) {
	router.Handle(eventbus.LogChannel, func(_ context.Context, payload []byte) {
		forwarder.Forward(payload)
	})
	logger.Info("log forward listener registered", "channel", eventbus.LogChannel)
}
