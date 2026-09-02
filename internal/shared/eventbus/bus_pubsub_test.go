package eventbus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// The Bus's Pub/Sub half — HandleNotify / HandleRequest / Notify / Request —
// is exercised over miniredis, the same go-redis adapter pointed at an
// in-process server (there is no transport port for this half). The stream
// side uses ChannelStreamTransport so one Run drives both.

func newBusOnRedis(t *testing.T, source string) *Bus {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	bus, err := New(Config{
		Redis:   client,
		Source:  source,
		Streams: ChannelStreamTransport(),
		Policy:  testBusPolicy,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return bus
}

func TestBusNotifyDeliversRawPayloadToEveryHandler(t *testing.T) {
	bus := newBusOnRedis(t, SourceServer)
	topic := AgentConfigChangedTopic("nas-01")

	got := make(chan string, 2)
	for i := range 2 {
		if err := bus.HandleNotify(topic, func(_ context.Context, payload []byte) {
			got <- fmt.Sprintf("h%d:%s", i, payload)
		}); err != nil {
			t.Fatalf("HandleNotify: %v", err)
		}
	}
	runBus(t, bus)

	if err := bus.Notify(context.Background(), topic, []byte("changed")); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	seen := map[string]bool{}
	for range 2 {
		select {
		case s := <-got:
			seen[s] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("only saw %v", seen)
		}
	}
	for _, want := range []string{"h0:changed", "h1:changed"} {
		if !seen[want] {
			t.Errorf("missing notify delivery %q; saw %v", want, seen)
		}
	}
}

func TestBusRequestReturnsReplyWithCorrelationAndReplyName(t *testing.T) {
	bus := newBusOnRedis(t, SourceServer)
	topic := HeartbeatTopic()

	if err := bus.HandleRequest(topic, func(_ context.Context, req *Event) ([]byte, error) {
		if req.GetName() != HeartbeatRequestEventName {
			t.Errorf("request name = %q, want %q", req.GetName(), HeartbeatRequestEventName)
		}
		return []byte(`{"ok":true}`), nil
	}); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	runBus(t, bus)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	reply, err := bus.Request(ctx, topic, HeartbeatRequestEventName, "corr-1", []byte(`{}`))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if reply.GetCorrelationId() != "corr-1" {
		t.Errorf("reply correlation_id = %q, want corr-1", reply.GetCorrelationId())
	}
	if reply.GetName() != HeartbeatReplyEventName {
		t.Errorf("reply name = %q, want %q (the topic's ReplyName)", reply.GetName(), HeartbeatReplyEventName)
	}
	if reply.GetSource() != SourceServer {
		t.Errorf("reply source = %q, want %q (stamped from Config)", reply.GetSource(), SourceServer)
	}
	if string(reply.GetPayload()) != `{"ok":true}` {
		t.Errorf("reply payload = %s", reply.GetPayload())
	}
}

func TestBusRequestMintsCorrelationIDWhenEmpty(t *testing.T) {
	bus := newBusOnRedis(t, SourceServer)
	topic := HeartbeatTopic()

	responderSaw := make(chan string, 1)
	if err := bus.HandleRequest(topic, func(_ context.Context, req *Event) ([]byte, error) {
		responderSaw <- req.GetCorrelationId()
		return []byte(`{}`), nil
	}); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	runBus(t, bus)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	reply, err := bus.Request(ctx, topic, HeartbeatRequestEventName, "", nil)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if reply.GetCorrelationId() == "" {
		t.Fatal("Request returned a reply with an empty correlation_id; one should have been minted")
	}
	select {
	case saw := <-responderSaw:
		if saw != reply.GetCorrelationId() {
			t.Errorf("responder saw correlation_id %q, reply carried %q — the minted id must be the same one", saw, reply.GetCorrelationId())
		}
	case <-time.After(time.Second):
		t.Fatal("responder never ran")
	}
}

func TestBusRequestWithNoResponderReturnsErrNoResponder(t *testing.T) {
	bus := newBusOnRedis(t, SourceServer)
	runBus(t, bus) // nothing registered on the request channel

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := bus.Request(ctx, HeartbeatTopic(), HeartbeatRequestEventName, "corr-none", nil)
	if !errors.Is(err, ErrNoResponder) {
		t.Fatalf("err = %v, want ErrNoResponder", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, must still wrap context.DeadlineExceeded", err)
	}
}

func TestBusRequestHandlerErrorSendsNoReply(t *testing.T) {
	bus := newBusOnRedis(t, SourceServer)
	if err := bus.HandleRequest(HeartbeatTopic(), func(context.Context, *Event) ([]byte, error) {
		return nil, errUnreachable
	}); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	runBus(t, bus)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := bus.Request(ctx, HeartbeatTopic(), HeartbeatRequestEventName, "corr-err", nil)
	if !errors.Is(err, ErrNoResponder) {
		t.Fatalf("err = %v, want ErrNoResponder (a handler error must send no reply)", err)
	}
}

func TestBusRequestHandlerNilPayloadSendsNoReply(t *testing.T) {
	bus := newBusOnRedis(t, SourceServer)
	if err := bus.HandleRequest(HeartbeatTopic(), func(context.Context, *Event) ([]byte, error) {
		return nil, nil
	}); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	runBus(t, bus)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := bus.Request(ctx, HeartbeatTopic(), HeartbeatRequestEventName, "corr-nil", nil)
	if !errors.Is(err, ErrNoResponder) {
		t.Fatalf("err = %v, want ErrNoResponder (a nil payload must send no reply)", err)
	}
}

// One Run drives the Watermill stream router and the Pub/Sub receive loops
// together, and Ready does not close until both are live.
func TestBusRunDrivesStreamAndPubSubUnderOneCall(t *testing.T) {
	bus := newBusOnRedis(t, SourceServer)

	streamGot := make(chan string, 1)
	if err := bus.HandleStream(SystemConfigUpdateTopic(), map[string]StreamHandler{
		SystemConfigUpdateEventName: func(_ context.Context, e *Event) error {
			streamGot <- e.GetCorrelationId()
			return nil
		},
	}); err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	notifyGot := make(chan string, 1)
	if err := bus.HandleNotify(LogTopic(), func(_ context.Context, payload []byte) {
		notifyGot <- string(payload)
	}); err != nil {
		t.Fatalf("HandleNotify: %v", err)
	}
	runBus(t, bus)

	ctx := context.Background()
	if err := bus.Publish(ctx, SystemConfigUpdateTopic(), SystemConfigUpdateEventName, "s1", []byte(`{}`)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := bus.Notify(ctx, LogTopic(), []byte("log-line")); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	select {
	case id := <-streamGot:
		if id != "s1" {
			t.Errorf("stream handler saw %q, want s1", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream handler never ran under the shared Run")
	}
	select {
	case line := <-notifyGot:
		if line != "log-line" {
			t.Errorf("notify handler saw %q, want log-line", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("notify handler never ran under the shared Run")
	}
}

// --- validation, no Run needed ----------------------------------------------

func TestBusNotifyRejectsWrongKind(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	if err := bus.Notify(context.Background(), HeartbeatTopic(), nil); !errors.Is(err, ErrWrongKind) {
		t.Fatalf("err = %v, want ErrWrongKind", err)
	}
}

func TestBusRequestRejectsWrongKind(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	_, err := bus.Request(context.Background(), LogTopic(), "whatever", "corr", nil)
	if !errors.Is(err, ErrWrongKind) {
		t.Fatalf("err = %v, want ErrWrongKind", err)
	}
}

func TestBusRequestRejectsOffTableName(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	_, err := bus.Request(context.Background(), HeartbeatTopic(), "not.a.real.event", "corr", nil)
	if !errors.Is(err, ErrUnknownEvent) {
		t.Fatalf("err = %v, want ErrUnknownEvent", err)
	}
}

func TestBusHandleNotifyRejectsWrongKind(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	err := bus.HandleNotify(HeartbeatTopic(), func(context.Context, []byte) {})
	if !errors.Is(err, ErrWrongKind) {
		t.Fatalf("err = %v, want ErrWrongKind", err)
	}
}

func TestBusHandleRequestRejectsWrongKind(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	err := bus.HandleRequest(LogTopic(), func(context.Context, *Event) ([]byte, error) { return nil, nil })
	if !errors.Is(err, ErrWrongKind) {
		t.Fatalf("err = %v, want ErrWrongKind", err)
	}
}

func TestBusHandleRequestRequiresReplyName(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	topic := Topic{Name: "x.request", Kind: KindRequestReply, Events: []string{"x.req"}}
	err := bus.HandleRequest(topic, func(context.Context, *Event) ([]byte, error) { return nil, nil })
	if err == nil || errors.Is(err, ErrWrongKind) {
		t.Fatalf("err = %v, want a missing-ReplyName error", err)
	}
}

func TestBusHandleRequestRejectsDuplicateResponder(t *testing.T) {
	bus := newBusOnRedis(t, SourceServer)
	h := func(context.Context, *Event) ([]byte, error) { return nil, nil }
	if err := bus.HandleRequest(HeartbeatTopic(), h); err != nil {
		t.Fatalf("first HandleRequest: %v", err)
	}
	if err := bus.HandleRequest(HeartbeatTopic(), h); err == nil {
		t.Fatal("second HandleRequest on the same topic should be rejected")
	}
}

func TestBusPubSubRegistrationAfterRunIsRejected(t *testing.T) {
	bus := newBusOnRedis(t, SourceServer)
	if err := bus.HandleNotify(LogTopic(), func(context.Context, []byte) {}); err != nil {
		t.Fatalf("HandleNotify: %v", err)
	}
	runBus(t, bus)

	if err := bus.HandleNotify(AgentConfigChangedTopic("x"), func(context.Context, []byte) {}); !errors.Is(err, ErrBusRunning) {
		t.Errorf("HandleNotify after Run: err = %v, want ErrBusRunning", err)
	}
	if err := bus.HandleRequest(HeartbeatTopic(), func(context.Context, *Event) ([]byte, error) { return nil, nil }); !errors.Is(err, ErrBusRunning) {
		t.Errorf("HandleRequest after Run: err = %v, want ErrBusRunning", err)
	}
}
