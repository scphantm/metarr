// Package eventbus implements the two Redis-backed messaging mechanisms
// used by this service:
//
//   - PubSubBus: an ephemeral, low-latency channel used for the heartbeat's
//     blocking request/reply exchange.
//   - StreamBus: a durable, consumer-group-based event bus (Redis Streams)
//     used for fire-and-forget background job events, so an event isn't
//     lost even if no listener is running at the moment it's published.
//
// Every message carries a correlation ID so it can be traced end to end
// across both mechanisms.
package eventbus

import (
	"encoding/json"
	"time"
)

// Event is the envelope for every message that flows through the event bus,
// whether over Pub/Sub or Streams.
type Event struct {
	CorrelationID string          `json:"correlation_id"`
	Name          string          `json:"name"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Timestamp     time.Time       `json:"timestamp"`
}
