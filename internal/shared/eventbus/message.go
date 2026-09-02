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
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	busv1 "Metarr/internal/genproto/metarr/bus/v1"
)

// Event is the envelope for every message that flows through the event bus,
// whether over Pub/Sub or Streams. It is the generated metarr.bus.v1.EventEnvelope
// proto message (docs/adr/0005, docs/adr/0006, docs/adr/0008): there is no
// hand-written mirror to keep in step with the wire form.
type Event = busv1.EventEnvelope

// Source values for the envelope's Source field. They match the logging
// `source` convention: one fixed value for the server, one per agent slug.
const SourceServer = "metarr-server"

// agentSourcePrefix is the fixed lead of every AgentSource value.
const agentSourcePrefix = "metarr-agent-"

// AgentSource is the envelope Source for events published by the agent slug.
func AgentSource(slug string) string { return agentSourcePrefix + slug }

// SlugFromAgentSource recovers the slug from a Source produced by
// AgentSource, reporting false for SourceServer or any other value. The
// dashboard's expected-vs-actual check uses it to map an expected agent
// identity back to the presence key that says whether that agent is here.
func SlugFromAgentSource(source string) (string, bool) {
	if !strings.HasPrefix(source, agentSourcePrefix) || source == agentSourcePrefix {
		return "", false
	}
	return source[len(agentSourcePrefix):], true
}

// NewEvent builds an envelope stamped with the current time. source is
// SourceServer or AgentSource(slug); payload is the already-encoded inner
// message, or nil for an event that carries none.
func NewEvent(source, name, correlationID string, payload []byte) *Event {
	return &Event{
		Name:          name,
		Source:        source,
		CorrelationId: correlationID,
		Timestamp:     timestamppb.Now(),
		Payload:       payload,
	}
}

// eventMarshal / eventUnmarshal encode the envelope exactly as the stored
// config is encoded (appconfig.MarshalStored): protojson with proto field
// names and unpopulated fields emitted. Using one encoding on both the
// publish and the consume side is what removes the old system_config_update
// asymmetry — it was published as protojson and read back with encoding/json
// — by construction.
var (
	eventMarshal   = protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}
	eventUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}
)

// MarshalEvent encodes event as the canonical bus JSON form.
func MarshalEvent(event *Event) ([]byte, error) {
	return eventMarshal.Marshal(event)
}

// UnmarshalEvent decodes bytes produced by MarshalEvent back into event.
func UnmarshalEvent(data []byte, event *Event) error {
	return eventUnmarshal.Unmarshal(data, event)
}
