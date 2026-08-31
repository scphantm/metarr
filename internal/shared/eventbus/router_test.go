package eventbus

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
)

// The Router seam is exercised entirely over watermill's in-memory
// gochannel: no Redis, no consumer groups, just the middleware stack and the
// registration helper doing what the acceptance criteria describe.

func testPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, BackoffBase: time.Millisecond, BackoffMax: 2 * time.Millisecond}
}

// newGoChannelRouter returns a Router whose dead-letter publisher and
// per-stream subscribers are all one gochannel, plus that gochannel so a
// test can observe DeadLetterStream directly.
func newGoChannelRouter(t *testing.T, policy RetryPolicy) (*Router, *gochannel.GoChannel) {
	t.Helper()

	logger := NewSlogAdapter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	pubSub := gochannel.NewGoChannel(gochannel.Config{}, logger)

	router, err := NewRouter(pubSub, func(_, _ string) (message.Subscriber, error) {
		return pubSub, nil
	}, policy, logger)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	t.Cleanup(func() {
		_ = router.Close()
		_ = pubSub.Close()
	})
	return router, pubSub
}

// runRouter starts the router and waits until its handlers are live, so a
// publish that follows is actually delivered.
func runRouter(t *testing.T, router *Router) context.CancelFunc {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- router.Run(ctx) }()

	select {
	case <-router.Running():
	case err := <-done:
		cancel()
		t.Fatalf("router stopped before running: %v", err)
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("router did not start within 2s")
	}

	t.Cleanup(cancel)
	return cancel
}

func publishEnvelope(t *testing.T, pub message.Publisher, stream string, event *Event) {
	t.Helper()

	data, err := MarshalEvent(event)
	if err != nil {
		t.Fatalf("MarshalEvent: %v", err)
	}
	if err := pub.Publish(stream, message.NewMessage("test-"+event.GetCorrelationId(), data)); err != nil {
		t.Fatalf("publish to %s: %v", stream, err)
	}
}

// receiveWithin returns the next message on ch, or fails if none arrives.
func receiveWithin(t *testing.T, ch <-chan *message.Message, d time.Duration, what string) *message.Message {
	t.Helper()

	select {
	case msg := <-ch:
		return msg
	case <-time.After(d):
		t.Fatalf("timed out waiting for %s", what)
		return nil
	}
}

func TestRouterDecodesEnvelopeAndDispatchesToHandler(t *testing.T) {
	router, pubSub := newGoChannelRouter(t, testPolicy())

	got := make(chan *Event, 1)
	if err := router.Handle("scan-results", AgentScanResultStream, AgentScanResultGroup, ConsumerName,
		func(_ context.Context, event *Event) error {
			got <- event
			return nil
		}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	runRouter(t, router)

	publishEnvelope(t, pubSub, AgentScanResultStream,
		NewEvent(AgentSource("nas-01"), AgentScanResultEventName, "corr-1", []byte(`{"scan_id":"corr-1"}`)))

	event := <-got
	if event.GetName() != AgentScanResultEventName {
		t.Errorf("handler got name %q, want %q", event.GetName(), AgentScanResultEventName)
	}
	if string(event.GetPayload()) != `{"scan_id":"corr-1"}` {
		t.Errorf("handler got payload %s", event.GetPayload())
	}
	if event.GetSource() != AgentSource("nas-01") {
		t.Errorf("handler got source %q", event.GetSource())
	}
}

func TestRouterRetriesThenParksOnDeadLetter(t *testing.T) {
	policy := testPolicy()
	router, pubSub := newGoChannelRouter(t, policy)

	deadLetters, err := pubSub.Subscribe(context.Background(), DeadLetterStream)
	if err != nil {
		t.Fatalf("subscribe dead-letter: %v", err)
	}

	var calls atomic.Int32
	if err := router.Handle("always-fails", SystemConfigUpdateStream, SystemConfigUpdateGroup, ConsumerName,
		func(_ context.Context, _ *Event) error {
			calls.Add(1)
			return errUnreachable
		}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	runRouter(t, router)

	publishEnvelope(t, pubSub, SystemConfigUpdateStream,
		NewEvent(SourceServer, SystemConfigUpdateEventName, "corr-2", []byte(`{}`)))

	parked := receiveWithin(t, deadLetters, 2*time.Second, "a message on the dead-letter stream")
	parked.Ack()

	if want := int32(policy.MaxAttempts + 1); calls.Load() != want {
		t.Errorf("handler ran %d times, want %d (first attempt + %d retries)", calls.Load(), want, policy.MaxAttempts)
	}

	var parkedEvent Event
	if err := UnmarshalEvent(parked.Payload, &parkedEvent); err != nil {
		t.Fatalf("dead-letter payload is not an envelope: %v", err)
	}
	if parkedEvent.GetCorrelationId() != "corr-2" {
		t.Errorf("parked the wrong message: correlation_id %q", parkedEvent.GetCorrelationId())
	}
	if reason := parked.Metadata.Get("reason_poisoned"); reason == "" {
		t.Error("parked message carries no reason_poisoned metadata")
	}

	// The source message must not still be cycling: give the router a moment
	// and confirm the call count has settled.
	settled := calls.Load()
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != settled {
		t.Errorf("handler kept being called after the message was parked: %d -> %d", settled, calls.Load())
	}
}

func TestRouterHandlerReturningNilNeverParks(t *testing.T) {
	router, pubSub := newGoChannelRouter(t, testPolicy())

	deadLetters, err := pubSub.Subscribe(context.Background(), DeadLetterStream)
	if err != nil {
		t.Fatalf("subscribe dead-letter: %v", err)
	}

	handled := make(chan struct{}, 1)
	if err := router.Handle("ran-and-failed", AgentScanResultStream, AgentScanResultGroup, ConsumerName,
		func(_ context.Context, _ *Event) error {
			// Simulates a handler whose work failed: it publishes a failure
			// result event (not modelled here) and returns nil.
			handled <- struct{}{}
			return nil
		}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	runRouter(t, router)

	publishEnvelope(t, pubSub, AgentScanResultStream,
		NewEvent(AgentSource("nas-01"), AgentScanFailedEventName, "corr-3", []byte(`{}`)))

	<-handled
	select {
	case <-deadLetters:
		t.Fatal("a handler that returned nil must never reach the dead-letter stream")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRouterUndecodableEnvelopeIsParked(t *testing.T) {
	router, pubSub := newGoChannelRouter(t, testPolicy())

	deadLetters, err := pubSub.Subscribe(context.Background(), DeadLetterStream)
	if err != nil {
		t.Fatalf("subscribe dead-letter: %v", err)
	}

	called := make(chan struct{}, 1)
	if err := router.Handle("never-runs", SystemConfigUpdateStream, SystemConfigUpdateGroup, ConsumerName,
		func(_ context.Context, _ *Event) error {
			called <- struct{}{}
			return nil
		}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	runRouter(t, router)

	if err := pubSub.Publish(SystemConfigUpdateStream, message.NewMessage("bad", []byte("this is not an envelope"))); err != nil {
		t.Fatalf("publish: %v", err)
	}

	parked := receiveWithin(t, deadLetters, 2*time.Second, "the undecodable message to be parked")
	parked.Ack()

	select {
	case <-called:
		t.Error("handler ran on a payload that is not a valid envelope")
	default:
	}
}

var errUnreachable = &busError{"datastore unreachable"}

type busError struct{ msg string }

func (e *busError) Error() string { return e.msg }
