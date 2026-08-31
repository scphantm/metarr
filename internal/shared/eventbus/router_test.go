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

// newGoChannelRouter returns a Router whose per-stream subscribers are one
// gochannel, plus that gochannel so a test can publish onto a stream.
func newGoChannelRouter(t *testing.T, policy RetryPolicy) (*Router, *gochannel.GoChannel) {
	t.Helper()

	logger := NewSlogAdapter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	pubSub := gochannel.NewGoChannel(gochannel.Config{}, logger)

	router, err := NewRouter(func(_, _ string) (message.Subscriber, error) {
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

// A handler that always errors is retried the policy's number of times and
// then acked (dropped) — not redelivered forever, and with no parking
// stream. This covers the drop-after-retry middleware that replaced
// PoisonQueue.
func TestRouterDropsMessageAfterRetriesExhausted(t *testing.T) {
	policy := testPolicy()
	router, pubSub := newGoChannelRouter(t, policy)

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

	want := int32(policy.MaxAttempts + 1)
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() < want && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() != want {
		t.Fatalf("handler ran %d times, want %d (first attempt + %d retries)", calls.Load(), want, policy.MaxAttempts)
	}

	// The message must not still be cycling: give the router a moment and
	// confirm the call count has settled at the retry budget.
	settled := calls.Load()
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != settled {
		t.Errorf("handler kept being called after retries were spent: %d -> %d", settled, calls.Load())
	}
}

// A handler that returns nil is acked on the first attempt and never
// retried.
func TestRouterHandlerReturningNilIsNotRetried(t *testing.T) {
	router, pubSub := newGoChannelRouter(t, testPolicy())

	var calls atomic.Int32
	if err := router.Handle("ran-and-succeeded", AgentScanResultStream, AgentScanResultGroup, ConsumerName,
		func(_ context.Context, _ *Event) error {
			calls.Add(1)
			return nil
		}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	runRouter(t, router)

	publishEnvelope(t, pubSub, AgentScanResultStream,
		NewEvent(AgentSource("nas-01"), AgentScanFailedEventName, "corr-3", []byte(`{}`)))

	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("handler ran %d times, want exactly 1", got)
	}
}

// An undecodable payload never reaches the handler; it is retried and then
// dropped by the middleware, not dispatched.
func TestRouterUndecodableEnvelopeIsNotDispatched(t *testing.T) {
	router, pubSub := newGoChannelRouter(t, testPolicy())

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

	select {
	case <-called:
		t.Error("handler ran on a payload that is not a valid envelope")
	case <-time.After(200 * time.Millisecond):
	}
}

var errUnreachable = &busError{"datastore unreachable"}

type busError struct{ msg string }

func (e *busError) Error() string { return e.msg }
