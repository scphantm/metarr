package eventbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// Seam 1 (docs/adr/0006): the PubSubRouter driven through its public interface
// over an in-memory Redis. miniredis is the honest substitute here for the
// same reason it is for the Request/Reply test — the router uses
// SUBSCRIBE-acknowledgement semantics a watermill gochannel does not model.

// newPubSubRouterHarness stands up a miniredis, a router stamping source, and a
// plain PubSubBus for the caller side, all over the one client.
func newPubSubRouterHarness(t *testing.T, source string) (*PubSubRouter, *PubSubBus) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewPubSubRouter(client, source, discardSlog()), NewPubSubBus(client)
}

// startPubSubRouter starts the router and returns once Running() has fired. It
// fails the test if Run returns an error or never comes up.
func startPubSubRouter(t *testing.T, router *PubSubRouter) (context.Context, context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- router.Run(ctx) }()
	select {
	case <-router.Running():
	case err := <-done:
		cancel()
		t.Fatalf("router stopped before it came up: %v", err)
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("router never signalled Running()")
	}
	return ctx, cancel, done
}

func TestPubSubRouterHandleReceivesRawPayload(t *testing.T) {
	router, bus := newPubSubRouterHarness(t, SourceServer)
	const channel = "agent.config.changed.nas-01"

	got := make(chan []byte, 1)
	router.Handle(channel, func(_ context.Context, payload []byte) {
		got <- payload
	})

	ctx, cancel, _ := startPubSubRouter(t, router)
	defer cancel()

	// A notification carries whatever the publisher put on the wire — here a
	// bare string, not an envelope — and it must reach the handler untouched.
	if err := bus.client.Publish(ctx, channel, "reload-now").Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case payload := <-got:
		if string(payload) != "reload-now" {
			t.Errorf("handler got %q, want %q", payload, "reload-now")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler never received the published payload")
	}
}

func TestPubSubRouterRespondStampsTheReplyEnvelope(t *testing.T) {
	router, caller := newPubSubRouterHarness(t, AgentSource("nas-01"))
	const channel = "agent.nas-01.request"

	gotRequest := make(chan *Event, 1)
	router.Respond(channel, AgentNFOReadReplyEventName,
		func(_ context.Context, request *Event) ([]byte, error) {
			gotRequest <- request
			return []byte(`{"ok":true}`), nil
		})

	_, cancel, _ := startPubSubRouter(t, router)
	defer cancel()

	ctx, cancelReq := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelReq()

	reply, err := caller.Request(ctx, channel,
		NewEvent(SourceServer, AgentNFOReadEventName, "corr-1", []byte(`{"path":"movie.nfo"}`)))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	// The handler saw the decoded request envelope.
	select {
	case request := <-gotRequest:
		if request.GetName() != AgentNFOReadEventName {
			t.Errorf("handler request name = %q, want %q", request.GetName(), AgentNFOReadEventName)
		}
		if string(request.GetPayload()) != `{"path":"movie.nfo"}` {
			t.Errorf("handler request payload = %s", request.GetPayload())
		}
	default:
		t.Fatal("handler never received the request")
	}

	// The reply came back as an envelope with source, correlation_id, and the
	// registered reply name stamped by the router.
	if reply.GetSource() != AgentSource("nas-01") {
		t.Errorf("reply source = %q, want %q", reply.GetSource(), AgentSource("nas-01"))
	}
	if reply.GetCorrelationId() != "corr-1" {
		t.Errorf("reply correlation_id = %q, want corr-1", reply.GetCorrelationId())
	}
	if reply.GetName() != AgentNFOReadReplyEventName {
		t.Errorf("reply name = %q, want %q", reply.GetName(), AgentNFOReadReplyEventName)
	}
	if string(reply.GetPayload()) != `{"ok":true}` {
		t.Errorf("reply payload = %s", reply.GetPayload())
	}
}

func TestPubSubRouterRespondNilPayloadSendsNoReply(t *testing.T) {
	router, caller := newPubSubRouterHarness(t, SourceServer)
	const channel = "heartbeat.request"

	router.Respond(channel, HeartbeatReplyEventName,
		func(_ context.Context, _ *Event) ([]byte, error) {
			return nil, nil
		})

	_, cancel, _ := startPubSubRouter(t, router)
	defer cancel()

	ctx, cancelReq := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelReq()

	_, err := caller.Request(ctx, channel, NewEvent(SourceServer, "heartbeat.request", "corr-nil", nil))
	if !errors.Is(err, ErrNoResponder) {
		t.Fatalf("error %v is not ErrNoResponder", err)
	}
}

func TestPubSubRouterRespondErrorSendsNoReply(t *testing.T) {
	router, caller := newPubSubRouterHarness(t, SourceServer)
	const channel = "heartbeat.request"

	router.Respond(channel, HeartbeatReplyEventName,
		func(_ context.Context, _ *Event) ([]byte, error) {
			return []byte(`{"ignored":true}`), errors.New("handler blew up")
		})

	_, cancel, _ := startPubSubRouter(t, router)
	defer cancel()

	ctx, cancelReq := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelReq()

	_, err := caller.Request(ctx, channel, NewEvent(SourceServer, "heartbeat.request", "corr-err", nil))
	if !errors.Is(err, ErrNoResponder) {
		t.Fatalf("error %v is not ErrNoResponder", err)
	}
}

func TestPubSubRouterTwoHandlersOnOneChannelBothReceive(t *testing.T) {
	router, bus := newPubSubRouterHarness(t, SourceServer)
	const channel = "logs.app"

	first := make(chan []byte, 1)
	second := make(chan []byte, 1)
	router.Handle(channel, func(_ context.Context, p []byte) { first <- p })
	router.Handle(channel, func(_ context.Context, p []byte) { second <- p })

	ctx, cancel, _ := startPubSubRouter(t, router)
	defer cancel()

	if err := bus.client.Publish(ctx, channel, "one-line").Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}

	for name, ch := range map[string]chan []byte{"first": first, "second": second} {
		select {
		case p := <-ch:
			if string(p) != "one-line" {
				t.Errorf("%s handler got %q", name, p)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s handler never received the message", name)
		}
	}
}

func TestPubSubRouterRunStopsAndClosesSubscriptionsOnCancel(t *testing.T) {
	router, bus := newPubSubRouterHarness(t, SourceServer)
	const channel = "agent.config.changed.nas-01"

	received := make(chan struct{}, 8)
	router.Handle(channel, func(_ context.Context, _ []byte) { received <- struct{}{} })

	ctx, cancel, done := startPubSubRouter(t, router)

	// It is live: a publish reaches the handler.
	if err := bus.client.Publish(ctx, channel, "before").Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never received the pre-cancel message")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil on cancel", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}

	// The subscription is closed: a fresh publish reaches nobody. Use a new
	// context — the Run one is cancelled.
	if err := bus.client.Publish(context.Background(), channel, "after").Err(); err != nil {
		t.Fatalf("publish after cancel: %v", err)
	}
	select {
	case <-received:
		t.Fatal("handler received a message after Run stopped")
	case <-time.After(300 * time.Millisecond):
	}
}
