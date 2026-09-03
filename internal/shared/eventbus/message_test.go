package eventbus

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
)

// The external contract (docs/adr/0006): a stream entry carries the envelope
// as JSON in its payload, with proto field names, so an integrator needs only
// a Redis client and a JSON parser. Pin that shape.
func TestMarshalEventProducesTheDocumentedJSONShape(t *testing.T) {
	event := newEnvelope(SourceServer, "agent.scan_result", "corr-1", []byte(`{"logging":{"server_level":"debug"}}`))

	data, err := MarshalEvent(event)
	if err != nil {
		t.Fatalf("MarshalEvent: %v", err)
	}

	var shape map[string]json.RawMessage
	if err := json.Unmarshal(data, &shape); err != nil {
		t.Fatalf("envelope is not valid JSON: %v\n%s", err, data)
	}
	for _, key := range []string{"name", "source", "correlation_id", "timestamp", "payload"} {
		if _, ok := shape[key]; !ok {
			t.Errorf("envelope JSON is missing the %q field: %s", key, data)
		}
	}
	if _, camel := shape["correlationId"]; camel {
		t.Errorf("envelope used camelCase; UseProtoNames is not in effect: %s", data)
	}
}

// A protojson-out / encoding/json-back mismatch mishandles well-known types
// like timestamps. One encoding on both sides removes that by construction —
// MarshalEvent's output must feed straight back into UnmarshalEvent.
func TestEventRoundTripsThroughTheBusEncoding(t *testing.T) {
	original := newEnvelope(AgentSource("nas-01"), "agent.scan_result", "corr-42", []byte(`{"scan_id":"corr-42"}`))

	data, err := MarshalEvent(original)
	if err != nil {
		t.Fatalf("MarshalEvent: %v", err)
	}

	var readBack Event
	if err := UnmarshalEvent(data, &readBack); err != nil {
		t.Fatalf("UnmarshalEvent: %v", err)
	}
	if !proto.Equal(original, &readBack) {
		t.Errorf("round trip changed the envelope:\n got %v\nwant %v", &readBack, original)
	}
	if string(readBack.Payload) != `{"scan_id":"corr-42"}` {
		t.Errorf("payload bytes changed: %s", readBack.Payload)
	}
}

func TestAgentSourceFollowsTheLoggingConvention(t *testing.T) {
	if got, want := AgentSource("nas-01"), "metarr-agent-nas-01"; got != want {
		t.Errorf("AgentSource = %q, want %q", got, want)
	}
	if SourceServer != "metarr-server" {
		t.Errorf("SourceServer = %q, want %q", SourceServer, "metarr-server")
	}
}
