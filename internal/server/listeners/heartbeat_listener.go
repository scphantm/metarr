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
// Redis round-trip health check on router. For every request on the heartbeat
// request channel it answers on that request's correlation-scoped reply
// channel with the current time and correlation ID — the reply the heartbeat
// handler is blocked waiting on. The router owns the subscription loop and
// stamps source (metarr-server), correlation ID, and the reply event name.
// Both ends run in metarr-server; a completed round trip proves the server's
// Redis request/reply path, not that any agent is reachable.
func RegisterHeartbeatResponder(router *eventbus.PubSubRouter, logger *slog.Logger) {
	router.Respond(eventbus.HeartbeatRequestChannel, eventbus.HeartbeatReplyEventName,
		func(_ context.Context, request *eventbus.Event) ([]byte, error) {
			payload, err := buildHeartbeatReply(request.GetCorrelationId(), time.Now())
			if err != nil {
				logger.Error("heartbeat responder: failed to marshal reply",
					"correlation_id", request.GetCorrelationId(), "error", err)
				return nil, err
			}
			return payload, nil
		})
	logger.Info("heartbeat responder registered", "channel", eventbus.HeartbeatRequestChannel)
}
