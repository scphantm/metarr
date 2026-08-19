package listeners

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"Metarr/internal/shared/eventbus"
)

type heartbeatReply struct {
	Time          string `json:"time"`
	CorrelationID string `json:"correlation_id"`
}

// RunHeartbeatListener subscribes to the heartbeat request channel and, for
// every request it sees, replies on that request's correlation-scoped reply
// channel with the current time and the correlation ID — the reply the
// heartbeat handler is blocked waiting on.
func RunHeartbeatListener(ctx context.Context, bus *eventbus.PubSubBus, logger *slog.Logger) {
	sub := bus.Subscribe(ctx, eventbus.HeartbeatRequestChannel)
	defer sub.Close()

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
			if err := json.Unmarshal([]byte(msg.Payload), &requestEvent); err != nil {
				logger.Error("heartbeat listener: invalid request payload", "error", err)
				continue
			}

			now := time.Now().UTC()
			payload, err := json.Marshal(heartbeatReply{
				Time:          now.Format(time.RFC3339),
				CorrelationID: requestEvent.CorrelationID,
			})
			if err != nil {
				logger.Error("heartbeat listener: failed to marshal reply", "correlation_id", requestEvent.CorrelationID, "error", err)
				continue
			}

			replyEvent := eventbus.Event{
				CorrelationID: requestEvent.CorrelationID,
				Name:          "heartbeat.reply",
				Payload:       payload,
				Timestamp:     now,
			}

			if err := bus.Reply(ctx, requestEvent.CorrelationID, replyEvent); err != nil {
				logger.Error("heartbeat listener: failed to publish reply", "correlation_id", requestEvent.CorrelationID, "error", err)
			}
		}
	}
}
