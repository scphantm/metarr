package listeners

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/version"
)

// buildHeartbeatReply is the pure half of the heartbeat responder: the reply
// envelope stamping (source, correlation ID, reply name) is observed through
// the Bus request/reply seam, so here we only pin the payload shape.

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

// The heartbeat health check works end to end through the Bus: the responder
// registered by RegisterHeartbeatResponder answers a bus.Request on the
// heartbeat request/reply topic, and the reply echoes the request's
// correlation ID and carries the topic's reply event name.
func TestRegisterHeartbeatResponderAnswersThroughTheBus(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus, err := eventbus.New(eventbus.Config{
		Source:  eventbus.SourceServer,
		Streams: eventbus.ChannelStreamTransport(),
		PubSub:  eventbus.InMemoryPubSub(),
		Policy:  eventbus.DefaultBusPolicy,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("eventbus.New: %v", err)
	}
	if err := RegisterHeartbeatResponder(bus, logger); err != nil {
		t.Fatalf("RegisterHeartbeatResponder: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- bus.Run(ctx) }()
	select {
	case <-bus.Ready():
	case err := <-runDone:
		t.Fatalf("bus stopped before ready: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("bus never became ready")
	}
	t.Cleanup(func() {
		cancel()
		<-runDone
		_ = bus.Close()
	})

	reqCtx, reqCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer reqCancel()

	reply, err := bus.Request(reqCtx, eventbus.HeartbeatTopic(),
		eventbus.HeartbeatRequestEventName, "corr-hb", nil)
	if err != nil {
		t.Fatalf("bus.Request: %v", err)
	}
	if reply.GetName() != eventbus.HeartbeatReplyEventName {
		t.Errorf("reply name = %q, want %q", reply.GetName(), eventbus.HeartbeatReplyEventName)
	}
	if reply.GetCorrelationId() != "corr-hb" {
		t.Errorf("reply correlation_id = %q, want corr-hb", reply.GetCorrelationId())
	}

	var body heartbeatReply
	if err := json.Unmarshal(reply.GetPayload(), &body); err != nil {
		t.Fatalf("unmarshal reply payload: %v", err)
	}
	if body.CorrelationID != "corr-hb" {
		t.Errorf("payload correlation_id = %q, want corr-hb", body.CorrelationID)
	}
	if body.Version != version.Raw {
		t.Errorf("payload version = %q, want %q", body.Version, version.Raw)
	}
}
