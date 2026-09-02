package eventbus

import (
	"encoding/json"
	"testing"
)

// The stream-entry framing is the cross-language contract (docs/adr/0006
// 2026-09-01, docs/adr/0008): one field, "payload", carrying the envelope
// JSON — no _watermill_message_uuid, no msgpack metadata blob.

func TestMinimalStreamValuesCarriesOnlyPayload(t *testing.T) {
	envelope := MarshalEventOrFatal(t, NewEvent(SourceServer, SystemConfigUpdateEventName, "corr-1", []byte(`{"k":1}`)))

	values := minimalStreamValues(envelope)

	if len(values) != 1 {
		t.Fatalf("stream entry has %d fields, want exactly 1: %v", len(values), values)
	}
	raw, ok := values["payload"]
	if !ok {
		t.Fatalf("stream entry has no %q field: %v", "payload", values)
	}
	if string(raw.([]byte)) != string(envelope) {
		t.Errorf("payload field = %s, want %s", raw, envelope)
	}
}

func TestPayloadFromStreamValuesRoundTrips(t *testing.T) {
	envelope := MarshalEventOrFatal(t, NewEvent(AgentSource("nas-01"), AgentScanResultEventName, "corr-2", []byte(`{}`)))

	// Redis hands entry values back as strings, not []byte.
	back, err := payloadFromStreamValues(map[string]any{"payload": string(envelope)})
	if err != nil {
		t.Fatalf("payloadFromStreamValues: %v", err)
	}
	if string(back) != string(envelope) {
		t.Errorf("round-trip = %s, want %s", back, envelope)
	}

	var event Event
	if err := UnmarshalEvent(back, &event); err != nil {
		t.Fatalf("recovered payload is not an envelope: %v", err)
	}
	if event.GetName() != AgentScanResultEventName {
		t.Errorf("recovered name %q", event.GetName())
	}
}

func TestPayloadFromStreamValuesRejectsMissingField(t *testing.T) {
	if _, err := payloadFromStreamValues(map[string]any{"metadata": "x"}); err == nil {
		t.Fatal("expected an error for a stream entry with no payload field")
	}
}

func TestMinimalUnmarshallerYieldsEnvelopePayload(t *testing.T) {
	envelope := MarshalEventOrFatal(t, NewEvent(SourceServer, SystemConfigUpdateEventName, "corr-3", []byte(`{"a":true}`)))

	msg, err := minimalUnmarshaller{}.Unmarshal(map[string]any{"payload": string(envelope)})
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if string(msg.Payload) != string(envelope) {
		t.Errorf("message payload = %s, want the envelope JSON %s", msg.Payload, envelope)
	}
	if msg.UUID == "" {
		t.Error("expected a freshly minted message UUID")
	}

	// And the entry really is minimal — no framing keys a reader would have
	// to know to skip.
	var onWire map[string]json.RawMessage
	if err := json.Unmarshal(mustJSON(t, minimalStreamValues(envelope)), &onWire); err != nil {
		t.Fatalf("re-encode entry: %v", err)
	}
	if _, framed := onWire["_watermill_message_uuid"]; framed {
		t.Error("stream entry leaked watermill's message-uuid framing field")
	}
	if _, framed := onWire["metadata"]; framed {
		t.Error("stream entry leaked a metadata framing field")
	}
}

// MarshalEventOrFatal encodes event or fails the test — the canonical bus
// JSON form, shared by the transport and bus tests.
func MarshalEventOrFatal(t *testing.T, event *Event) []byte {
	t.Helper()
	data, err := MarshalEvent(event)
	if err != nil {
		t.Fatalf("MarshalEvent: %v", err)
	}
	return data
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	// minimalStreamValues stores []byte; JSON-encode with the payload as a
	// string so the key set is what a test can inspect.
	m := map[string]string{}
	for k, val := range v.(map[string]any) {
		m[k] = string(val.([]byte))
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
