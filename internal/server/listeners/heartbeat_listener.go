package listeners

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/version"
)

type heartbeatReply struct {
	Time          string `json:"time"`
	CorrelationID string `json:"correlation_id"`
	Version       string `json:"version"`
}

// RunHeartbeatListener is the responder half of the heartbeat Redis
// round-trip health check. It subscribes to the heartbeat request channel
// and, for every request, replies on that request's correlation-scoped reply
// channel with the current time and correlation ID — the reply the heartbeat
// handler is blocked waiting on. Both ends run in metarr-server; a completed
// round trip proves the server's Redis request/reply path, not that any
// agent is reachable.
func RunHeartbeatListener(ctx context.Context, bus *eventbus.PubSubBus, logger *slog.Logger) {
	sub := bus.Subscribe(ctx, eventbus.HeartbeatRequestChannel)
	defer func() { _ = sub.Close() }()

	logger.Info("heartbeat listener started", "channel", eventbus.HeartbeatRequestChannel)

	for {
		select {
		case <-ctx.Done():
			logger.Info("heartbeat listener stopped")
			return
		case msg, ok := <-sub.Channel():
			if !ok {
				return
			}

			var requestEvent eventbus.Event
			if err := eventbus.UnmarshalEvent([]byte(msg.Payload), &requestEvent); err != nil {
				logger.Error("heartbeat listener: invalid request payload", "error", err)
				continue
			}

			now := time.Now().UTC()
			payload, err := json.Marshal(heartbeatReply{
				Time:          now.Format(time.RFC3339),
				CorrelationID: requestEvent.CorrelationId,
				Version:       version.Raw,
			})
			if err != nil {
				logger.Error("heartbeat listener: failed to marshal reply", "correlation_id", requestEvent.CorrelationId, "error", err)
				continue
			}

			replyEvent := eventbus.NewEvent(eventbus.SourceServer, "heartbeat.reply", requestEvent.CorrelationId, payload)

			if err := bus.Reply(ctx, requestEvent.CorrelationId, replyEvent); err != nil {
				logger.Error("heartbeat listener: failed to publish reply", "correlation_id", requestEvent.CorrelationId, "error", err)
			}
		}
	}
}
