package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"
)

// newStreamBusOnRedis returns a StreamBus and the client behind it, both
// bound to a fresh miniredis for the test.
func newStreamBusOnRedis(t *testing.T) (*StreamBus, redis.UniversalClient) {
	t.Helper()
	_, client := newRetentionRedis(t)
	bus, err := NewStreamBus(client, DefaultRetentionPolicy(), NewSlogAdapter(discardSlog()))
	if err != nil {
		t.Fatalf("NewStreamBus: %v", err)
	}
	return bus, client
}

// Publishing a known StreamTopic and consuming it through Router.Handle
// still round-trips: the same StreamTopic value drives both sides, and the
// envelope arrives intact (acceptance criterion, testing seam 2).
func TestStreamBusPublishRoundTripsThroughRouter(t *testing.T) {
	bus, client := newStreamBusOnRedis(t)

	// Short block/claim windows so a miniredis round trip completes in test
	// time rather than the library's multi-second defaults.
	newSub := func(group, consumer string) (message.Subscriber, error) {
		return redisstream.NewSubscriber(redisstream.SubscriberConfig{
			Client:        client,
			ConsumerGroup: group,
			Consumer:      consumer,
			BlockTime:     10 * time.Millisecond,
			ClaimInterval: 20 * time.Millisecond,
		}, NewSlogAdapter(discardSlog()))
	}

	router, err := NewRouter(newSub, testPolicy(), NewSlogAdapter(discardSlog()))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	t.Cleanup(func() { _ = router.Close() })

	topic := AgentScanResultTopic()
	got := make(chan *Event, 1)
	if err := router.Handle(topic, ConsumerName, func(_ context.Context, event *Event) error {
		got <- event
		return nil
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = router.Run(ctx) }()
	<-router.Running()

	want := NewEvent(AgentSource("nas-01"), AgentScanResultEventName, "corr-rt", []byte(`{"scan_id":"corr-rt"}`))
	if err := bus.Publish(ctx, topic, want); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case event := <-got:
		if event.GetName() != AgentScanResultEventName {
			t.Errorf("name = %q, want %q", event.GetName(), AgentScanResultEventName)
		}
		if event.GetCorrelationId() != "corr-rt" {
			t.Errorf("correlation_id = %q, want corr-rt", event.GetCorrelationId())
		}
		if string(event.GetPayload()) != `{"scan_id":"corr-rt"}` {
			t.Errorf("payload = %s", event.GetPayload())
		}
		if event.GetSource() != AgentSource("nas-01") {
			t.Errorf("source = %q", event.GetSource())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the published event was never consumed through Router.Handle")
	}
}

// A concrete per-agent command topic — a name the pattern row covers rather
// than a static row — is publishable: the guard resolves it through the
// pattern, not just the literal rows.
func TestStreamBusPublishAcceptsAPerAgentCommandTopic(t *testing.T) {
	bus, client := newStreamBusOnRedis(t)

	topic := AgentCommandTopic("nas-01")
	if err := bus.Publish(context.Background(), topic, NewEvent(SourceServer, AgentScanCommandEventName, "corr-cmd", []byte(`{}`))); err != nil {
		t.Fatalf("Publish to a per-agent command topic: %v", err)
	}
	if got := xlen(t, client, topic.Name); got != 1 {
		t.Errorf("%s length = %d, want 1", topic.Name, got)
	}
}

// A non-pattern StreamTopic whose Name is not one the stream topic table
// resolves to is rejected, and nothing is written (acceptance criterion).
func TestStreamBusPublishRejectsUnknownTopic(t *testing.T) {
	bus, client := newStreamBusOnRedis(t)

	unknown := StreamTopic{Name: "events.not_a_real_stream", Consumed: true}
	err := bus.Publish(context.Background(), unknown, NewEvent(SourceServer, "x", "corr-x", nil))
	if err == nil {
		t.Fatal("Publish to an unknown stream topic returned nil, want an error")
	}
	if got := xlen(t, client, unknown.Name); got != 0 {
		t.Errorf("%s length = %d, want 0 — a rejected publish must write nothing", unknown.Name, got)
	}
}

// A pattern StreamTopic is rejected: a glob names many streams, so it is not
// a publish target (acceptance criterion).
func TestStreamBusPublishRejectsPatternTopic(t *testing.T) {
	bus, _ := newStreamBusOnRedis(t)

	if err := bus.Publish(context.Background(), agentCommandStreamPatternTopic(), NewEvent(SourceServer, "x", "corr-x", nil)); err == nil {
		t.Fatal("Publish to a pattern topic returned nil, want an error")
	}
}
