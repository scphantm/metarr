package eventbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// The request/reply pair is exercised over a miniredis, not a real Redis
// process. It is a Pub/Sub-transport primitive by construction — it uses
// subscription-acknowledgement semantics a watermill gochannel does not
// model — so the honest in-memory substitute is miniredis, which speaks the
// same SUBSCRIBE/PUBLISH protocol.

func newPubSubBus(t *testing.T) *PubSubBus {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewPubSubBus(client)
}

// A responder that answers on the correlation-scoped reply channel: the
// heartbeat listener and the agent responder are both shaped like this.
func startResponder(t *testing.T, bus *PubSubBus, requestChannel string, reply func(req *Event) *Event) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	sub := bus.Subscribe(ctx, requestChannel)
	ready := make(chan struct{})
	go func() {
		if _, err := sub.Receive(ctx); err != nil {
			return
		}
		close(ready)
		for msg := range sub.Channel() {
			var req Event
			if err := UnmarshalEvent([]byte(msg.Payload), &req); err != nil {
				continue
			}
			_ = bus.Reply(ctx, req.GetCorrelationId(), reply(&req))
		}
	}()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("responder did not subscribe in time")
	}
}

func TestRequestReturnsTheResponderReply(t *testing.T) {
	bus := newPubSubBus(t)
	const channel = "agent.nas-01.request"

	startResponder(t, bus, channel, func(req *Event) *Event {
		return NewEvent(AgentSource("nas-01"), "agent.nfo_read", req.GetCorrelationId(), []byte(`{"ok":true}`))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	reply, err := bus.Request(ctx, channel,
		NewEvent(SourceServer, "agent.nfo_read", "corr-1", []byte(`{"path":"movie.nfo"}`)))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if reply.GetCorrelationId() != "corr-1" {
		t.Errorf("reply correlation_id = %q, want corr-1", reply.GetCorrelationId())
	}
	if string(reply.GetPayload()) != `{"ok":true}` {
		t.Errorf("reply payload = %s", reply.GetPayload())
	}
}

func TestRequestWithNoResponderReturnsErrNoResponder(t *testing.T) {
	bus := newPubSubBus(t)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_, err := bus.Request(ctx, "agent.nobody.request",
		NewEvent(SourceServer, "agent.nfo_read", "corr-2", nil))
	if err == nil {
		t.Fatal("expected an error when nothing answers")
	}
	if !errors.Is(err, ErrNoResponder) {
		t.Errorf("error %v is not ErrNoResponder", err)
	}
	// It still wraps the deadline error, so existing deadline checks keep working.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error %v does not wrap context.DeadlineExceeded", err)
	}
}

func TestReplyGoesToTheCorrelationScopedChannel(t *testing.T) {
	bus := newPubSubBus(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := bus.Subscribe(ctx, ReplyChannel("corr-3"))
	defer func() { _ = sub.Close() }()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if err := bus.Reply(ctx, "corr-3", NewEvent(SourceServer, "x", "corr-3", []byte("hi"))); err != nil {
		t.Fatalf("Reply: %v", err)
	}

	select {
	case msg := <-sub.Channel():
		var got Event
		if err := UnmarshalEvent([]byte(msg.Payload), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if string(got.GetPayload()) != "hi" {
			t.Errorf("payload = %s", got.GetPayload())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reply never arrived on the correlation-scoped channel")
	}
}
