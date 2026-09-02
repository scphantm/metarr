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

// buildHeartbeatReply is the pure heartbeat reply payload builder: given the
// request's correlation ID and the current time, it produces the JSON body
// the heartbeat handler is blocked waiting on — the current time, the
// correlation ID echoed back, and this server's version.
func buildHeartbeatReply(correlationID string, now time.Time) ([]byte, error) {
	return json.Marshal(heartbeatReply{
		Time:          now.UTC().Format(time.RFC3339),
		CorrelationID: correlationID,
		Version:       version.Raw,
	})
}

// RegisterHeartbeatResponder registers the responder half of the heartbeat
// Redis round-trip health check on the Bus. For every request on the
// heartbeat request/reply topic it answers with the current time and
// correlation ID — the reply the heartbeat handler is blocked waiting on. The
// Bus owns the subscription loop and stamps source (metarr-server),
// correlation ID, and the topic's reply event name. Both ends run in
// metarr-server; a completed round trip proves the server's Redis
// request/reply path, not that any agent is reachable.
func RegisterHeartbeatResponder(bus *eventbus.Bus, logger *slog.Logger) error {
	topic := eventbus.HeartbeatTopic()
	if err := bus.HandleRequest(topic,
		func(_ context.Context, request *eventbus.Event) ([]byte, error) {
			payload, err := buildHeartbeatReply(request.GetCorrelationId(), time.Now())
			if err != nil {
				logger.Error("heartbeat responder: failed to marshal reply",
					"correlation_id", request.GetCorrelationId(), "error", err)
				return nil, err
			}
			return payload, nil
		}); err != nil {
		return err
	}
	logger.Info("heartbeat responder registered", "channel", topic.Name)
	return nil
}
