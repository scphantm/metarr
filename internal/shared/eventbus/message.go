// Package eventbus is the one Redis-backed event bus for this service
// (docs/adr/0008). A single Bus type carries both delivery shapes:
//
//   - durable, consumer-group-based streams (Redis Streams) for
//     fire-and-forget background events, so an event isn't lost even if no
//     listener is running at the moment it's published; and
//   - ephemeral Pub/Sub channels for fire-and-forget notifications and the
//     one synchronous request/reply exchange (the heartbeat health check,
//     the agent NFO read).
//
// Every message carries a correlation ID so it can be traced end to end
// across both shapes.
package eventbus

import (
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

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
