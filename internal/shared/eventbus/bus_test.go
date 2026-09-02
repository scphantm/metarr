package eventbus

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// newEnvelope assembles an EventEnvelope for tests that publish straight to
// the transport or Redis — simulating another process (or another language)
// putting a well-formed envelope on the wire. Production code never
// hand-builds an envelope: Bus.Publish / Bus.Request / Bus.Notify stamp it.
func newEnvelope(source, name, correlationID string, payload []byte) *Event {
	return &Event{
		Name:          name,
		Source:        source,
		CorrelationId: correlationID,
		Timestamp:     timestamppb.Now(),
		Payload:       payload,
	}
}

// The Bus's durable-stream half is exercised over ChannelStreamTransport —
// no Redis, just the middleware stack and per-(topic, name) dispatch doing
// what the acceptance criteria describe. The one Redis-specific behaviour
// (consumer group create + XAUTOCLAIM reclaim) is in bus_miniredis_test.go.

func testPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, BackoffBase: time.Millisecond, BackoffMax: 2 * time.Millisecond}
}

// errUnreachable stands in for a "could not process at all" handler failure —
// the only case the failure convention says a handler should return an error
// for. Tests assert it survives the retry stack via errors.Is.
var errUnreachable = errors.New("datastore unreachable")

func testBusPolicy() BusPolicy {
	return BusPolicy{
		Retention:     RetentionPolicy{MaxLen: 1000, RetentionHours: 48},
		Retry:         testPolicy(),
		SweepInterval: time.Hour,
	}
}

// countingHandler is an slog.Handler that tallies records by level, so a
// test can assert the unknown-name default logs exactly once.
type countingHandler struct {
	warns atomic.Int32
}

func (h *countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *countingHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn {
		h.warns.Add(1)
	}
	return nil
}
func (h *countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(string) slog.Handler      { return h }

func newChannelBus(t *testing.T, source string, logger *slog.Logger) *Bus {
	t.Helper()
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	bus, err := New(Config{
		Source:  source,
		Streams: ChannelStreamTransport(),
		Policy:  testBusPolicy,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return bus
}

// runBus starts the bus and waits until its handlers are live, so a Publish
// that follows is actually delivered.
func runBus(t *testing.T, bus *Bus) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- bus.Run(ctx) }()

	select {
	case <-bus.Ready():
	case err := <-done:
		cancel()
		t.Fatalf("bus stopped before ready: %v", err)
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("bus did not become ready within 2s")
	}

	t.Cleanup(func() {
		cancel()
		<-done
		_ = bus.Close()
	})
}

func TestBusDispatchesPerTopicAndName(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)

	got := make(chan string, 3)
	err := bus.HandleStream(AgentScanResultTopic(), map[string]StreamHandler{
		AgentScanResultEventName:   func(_ context.Context, e *Event) error { got <- "result:" + e.GetCorrelationId(); return nil },
		AgentScanCompleteEventName: func(_ context.Context, e *Event) error { got <- "complete:" + e.GetCorrelationId(); return nil },
		AgentScanFailedEventName:   func(_ context.Context, e *Event) error { got <- "failed:" + e.GetCorrelationId(); return nil },
	})
	if err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	runBus(t, bus)

	ctx := context.Background()
	mustPublish(t, bus, AgentScanResultTopic(), AgentScanResultEventName, "a", []byte(`{}`))
	mustPublish(t, bus, AgentScanResultTopic(), AgentScanCompleteEventName, "b", []byte(`{}`))
	mustPublish(t, bus, AgentScanResultTopic(), AgentScanFailedEventName, "c", []byte(`{}`))
	_ = ctx

	seen := map[string]bool{}
	for range 3 {
		select {
		case s := <-got:
			seen[s] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("only saw %v", seen)
		}
	}
	for _, want := range []string{"result:a", "complete:b", "failed:c"} {
		if !seen[want] {
			t.Errorf("missing dispatch %q; saw %v", want, seen)
		}
	}
}

func TestBusStampsSourceFromConfig(t *testing.T) {
	bus := newChannelBus(t, AgentSource("nas-01"), nil)

	got := make(chan *Event, 1)
	if err := bus.HandleStream(AgentCommandTopic("nas-01"), map[string]StreamHandler{
		AgentScanCommandEventName: func(_ context.Context, e *Event) error { got <- e; return nil },
	}); err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	runBus(t, bus)

	mustPublish(t, bus, AgentCommandTopic("nas-01"), AgentScanCommandEventName, "corr", []byte(`{}`))

	select {
	case e := <-got:
		if e.GetSource() != AgentSource("nas-01") {
			t.Errorf("source = %q, want %q (stamped from Config, not a call arg)", e.GetSource(), AgentSource("nas-01"))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler never ran")
	}
}

// An event whose Name is on the stream but not in the handler map hits the
// unknown-name default: logged once, acked, handler never runs, no retry —
// and the stream keeps working for a Name that is mapped.
func TestBusUnknownNameDefaultLogsOnceAndAcks(t *testing.T) {
	counter := &countingHandler{}
	bus := newChannelBus(t, SourceServer, slog.New(counter))

	var mapped atomic.Int32
	if err := bus.HandleStream(AgentScanResultTopic(), map[string]StreamHandler{
		AgentScanResultEventName: func(_ context.Context, _ *Event) error { mapped.Add(1); return nil },
	}); err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	runBus(t, bus)

	// complete is a legal Name on the topic, so Publish accepts it, but no
	// handler is registered for it.
	mustPublish(t, bus, AgentScanResultTopic(), AgentScanCompleteEventName, "unmapped", []byte(`{}`))
	// then one that IS mapped, to prove the stream did not stall.
	mustPublish(t, bus, AgentScanResultTopic(), AgentScanResultEventName, "mapped", []byte(`{}`))

	deadline := time.Now().Add(2 * time.Second)
	for mapped.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)

	if mapped.Load() != 1 {
		t.Errorf("mapped handler ran %d times, want exactly 1", mapped.Load())
	}
	if counter.warns.Load() != 1 {
		t.Errorf("unknown-name default logged %d warnings, want exactly 1", counter.warns.Load())
	}
}

func TestBusRetriesThenDropsUnprocessableMessage(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)

	var calls atomic.Int32
	if err := bus.HandleStream(SystemConfigUpdateTopic(), map[string]StreamHandler{
		SystemConfigUpdateEventName: func(_ context.Context, _ *Event) error {
			calls.Add(1)
			return errUnreachable
		},
	}); err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	runBus(t, bus)

	mustPublish(t, bus, SystemConfigUpdateTopic(), SystemConfigUpdateEventName, "corr", []byte(`{}`))

	want := int32(testPolicy().MaxAttempts + 1)
	deadline := time.Now().Add(3 * time.Second)
	for calls.Load() < want && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() != want {
		t.Fatalf("handler ran %d times, want %d (first attempt + %d retries)", calls.Load(), want, testPolicy().MaxAttempts)
	}
	settled := calls.Load()
	time.Sleep(100 * time.Millisecond)
	if calls.Load() != settled {
		t.Errorf("message kept cycling after retries were spent: %d -> %d", settled, calls.Load())
	}
}

func TestBusBusinessFailureReturningNilIsNotRetried(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)

	var calls atomic.Int32
	if err := bus.HandleStream(AgentScanResultTopic(), map[string]StreamHandler{
		AgentScanFailedEventName: func(_ context.Context, _ *Event) error {
			calls.Add(1)
			return nil // ran, produced a business failure, reported elsewhere
		},
	}); err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	runBus(t, bus)

	mustPublish(t, bus, AgentScanResultTopic(), AgentScanFailedEventName, "corr", []byte(`{}`))

	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if calls.Load() != 1 {
		t.Errorf("handler ran %d times, want exactly 1", calls.Load())
	}
}

// --- validation, no Run needed ------------------------------------------

func TestBusPublishRejectsOffTableName(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	err := bus.Publish(context.Background(), SystemConfigUpdateTopic(), "not.a.real.event", "corr", nil)
	if !errors.Is(err, ErrUnknownEvent) {
		t.Fatalf("err = %v, want ErrUnknownEvent", err)
	}
}

// Wrong-Kind calls — Publish(LogTopic()), HandleStream(HeartbeatTopic()) and
// the rest — are no longer runtime errors to test: the verbs take
// StreamTopic / NotifyTopic / RequestTopic, so a mismatch does not compile.

func TestBusPublishRejectsPatternTopic(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	pattern := StreamTopic{Topic{
		Name:    AgentCommandStreamPattern,
		Kind:    KindStream,
		Pattern: true,
		Events:  []string{AgentScanCommandEventName},
	}}
	err := bus.Publish(context.Background(), pattern, AgentScanCommandEventName, "corr", nil)
	if !errors.Is(err, ErrNotPublishable) {
		t.Fatalf("err = %v, want ErrNotPublishable", err)
	}
}

func TestBusHandleStreamRejectsUnknownEventKey(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	err := bus.HandleStream(SystemConfigUpdateTopic(), map[string]StreamHandler{
		"bogus.event": func(context.Context, *Event) error { return nil },
	})
	if !errors.Is(err, ErrUnknownEvent) {
		t.Fatalf("err = %v, want ErrUnknownEvent", err)
	}
}

func TestBusRegistrationAfterRunIsRejected(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	if err := bus.HandleStream(SystemConfigUpdateTopic(), map[string]StreamHandler{
		SystemConfigUpdateEventName: func(context.Context, *Event) error { return nil },
	}); err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	runBus(t, bus)

	err := bus.HandleStream(AgentScanResultTopic(), map[string]StreamHandler{
		AgentScanResultEventName: func(context.Context, *Event) error { return nil },
	})
	if !errors.Is(err, ErrBusRunning) {
		t.Fatalf("err = %v, want ErrBusRunning", err)
	}
}

func TestBusSecondRunIsRejected(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	if err := bus.HandleStream(SystemConfigUpdateTopic(), map[string]StreamHandler{
		SystemConfigUpdateEventName: func(context.Context, *Event) error { return nil },
	}); err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	runBus(t, bus)

	if err := bus.Run(context.Background()); !errors.Is(err, ErrBusRunning) {
		t.Fatalf("second Run err = %v, want ErrBusRunning", err)
	}
}

// The stream publisher is live from New, so a Publish that races startup
// (the HTTP server accepts requests before bus.Run has spun up) succeeds
// rather than returning a "Publish before Run" error.
func TestBusPublishWorksBeforeRun(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	if err := bus.Publish(context.Background(), SystemConfigUpdateTopic(), SystemConfigUpdateEventName, "early", []byte(`{}`)); err != nil {
		t.Fatalf("Publish before Run: %v", err)
	}
}

// Ready() must not strand a waiter if the context is cancelled before the
// router ever reports itself running.
func TestBusReadyUnblocksOnEarlyCancel(t *testing.T) {
	bus := newChannelBus(t, SourceServer, nil)
	if err := bus.HandleStream(SystemConfigUpdateTopic(), map[string]StreamHandler{
		SystemConfigUpdateEventName: func(context.Context, *Event) error { return nil },
	}); err != nil {
		t.Fatalf("HandleStream: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done before Run
	done := make(chan error, 1)
	go func() { done <- bus.Run(ctx) }()

	select {
	case <-bus.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("Ready() never closed after an early-cancelled Run")
	}
	<-done
}

// failingSubscriberTransport is a stream transport whose Subscriber always
// errors, to exercise Run's setup-failure path.
type failingSubscriberTransport struct{ inner StreamTransport }

func (t failingSubscriberTransport) Publisher() (StreamPublisher, error) { return t.inner.Publisher() }
func (t failingSubscriberTransport) Subscriber(string, string) (message.Subscriber, error) {
	return nil, errUnreachable
}

// A Run that fails during setup (a subscriber that will not open) returns the
// error, closes Ready() so no waiter hangs, and leaves the bus un-started so
// a supervisor can retry rather than being permanently bricked.
func TestBusRunSetupFailureIsRecoverable(t *testing.T) {
	bus, err := New(Config{
		Source:  SourceServer,
		Streams: failingSubscriberTransport{inner: ChannelStreamTransport()},
		Policy:  testBusPolicy,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := bus.HandleStream(SystemConfigUpdateTopic(), map[string]StreamHandler{
		SystemConfigUpdateEventName: func(context.Context, *Event) error { return nil },
	}); err != nil {
		t.Fatalf("HandleStream: %v", err)
	}

	if err := bus.Run(context.Background()); !errors.Is(err, errUnreachable) {
		t.Fatalf("first Run err = %v, want the subscriber error", err)
	}
	select {
	case <-bus.Ready():
	case <-time.After(time.Second):
		t.Fatal("Ready() never closed after a failed Run")
	}
	if err := bus.Run(context.Background()); errors.Is(err, ErrBusRunning) {
		t.Error("a Run that failed during setup must leave the bus re-runnable, not ErrBusRunning")
	}
}

func mustPublish(t *testing.T, bus *Bus, topic StreamTopic, name, correlationID string, payload []byte) {
	t.Helper()
	if err := bus.Publish(context.Background(), topic, name, correlationID, payload); err != nil {
		t.Fatalf("Publish %s/%s: %v", topic.Name, name, err)
	}
}
