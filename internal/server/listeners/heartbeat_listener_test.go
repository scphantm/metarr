package listeners

import (
	"encoding/json"
	"testing"
	"time"

	"Metarr/internal/shared/version"
)

// buildHeartbeatReply is the pure half of the heartbeat responder: the reply
// envelope stamping (source, correlation ID, reply name) is observed through
// the PubSubRouter miniredis seam, so here we only pin the payload shape.

func TestBuildHeartbeatReplyEchoesCorrelationAndStampsTimeAndVersion(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 30, 0, 0, time.FixedZone("EST", -5*3600))

	payload, err := buildHeartbeatReply("corr-42", now)
	if err != nil {
		t.Fatalf("buildHeartbeatReply: %v", err)
	}

	var reply heartbeatReply
	if err := json.Unmarshal(payload, &reply); err != nil {
		t.Fatalf("unmarshal reply: %v", err)
	}

	if reply.CorrelationID != "corr-42" {
		t.Errorf("CorrelationID = %q, want corr-42", reply.CorrelationID)
	}
	if want := now.UTC().Format(time.RFC3339); reply.Time != want {
		t.Errorf("Time = %q, want %q (UTC, RFC3339)", reply.Time, want)
	}
	if reply.Version != version.Raw {
		t.Errorf("Version = %q, want %q", reply.Version, version.Raw)
	}
}

func TestBuildHeartbeatReplyIsValidJSON(t *testing.T) {
	payload, err := buildHeartbeatReply("", time.Now())
	if err != nil {
		t.Fatalf("buildHeartbeatReply: %v", err)
	}
	if !json.Valid(payload) {
		t.Fatalf("payload is not valid JSON: %s", payload)
	}
}
