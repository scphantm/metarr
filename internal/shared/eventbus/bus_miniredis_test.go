package eventbus

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// The Redis-Streams-specific behaviour of the Bus, on miniredis: the wire
// entry is exactly {"payload": <envelope JSON>}, the consumer group is
// created on first subscribe, and a message whose consumer died before
// acking is reclaimed by another consumer in the group.

func newRedisBus(t *testing.T, transport StreamTransport, client redis.UniversalClient, source string) *Bus {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus, err := New(Config{
		Redis:   client,
		Source:  source,
		Streams: transport,
		Policy:  testBusPolicy,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return bus
}

func TestBusOverRedisWireEntryIsPayloadOnly(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	transport := RedisStreamTransport(client, NewSlogAdapter(slog.New(slog.NewTextHandler(io.Discard, nil))))
	bus := newRedisBus(t, transport, client, SourceServer)
	if err := bus.HandleStream(SystemConfigUpdateTopic(), map[string]StreamHandler{
		SystemConfigUpdateEventName: func(context.Context, *Event) error { return nil },
	}); err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	runBus(t, bus)

	if err := bus.Publish(context.Background(), SystemConfigUpdateTopic(), SystemConfigUpdateEventName, "corr-wire", []byte(`{"hello":"world"}`)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	entries, err := client.XRange(context.Background(), SystemConfigUpdateStream, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("stream has %d entries, want 1", len(entries))
	}
	values := entries[0].Values
	if len(values) != 1 {
		t.Fatalf("entry has %d fields, want exactly 1 (no library framing): %v", len(values), values)
	}
	raw, ok := values["payload"]
	if !ok {
		t.Fatalf("entry has no %q field: %v", "payload", values)
	}
	var event Event
	if err := UnmarshalEvent([]byte(raw.(string)), &event); err != nil {
		t.Fatalf("payload field is not an envelope: %v", err)
	}
	if event.GetSource() != SourceServer || event.GetName() != SystemConfigUpdateEventName || event.GetCorrelationId() != "corr-wire" {
		t.Errorf("envelope = %+v", &event)
	}

	// The group the listener reads with was created on first subscribe.
	groups, err := client.XInfoGroups(context.Background(), SystemConfigUpdateStream).Result()
	if err != nil {
		t.Fatalf("XInfoGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != SystemConfigUpdateGroup {
		t.Fatalf("want group %q created on subscribe, got %+v", SystemConfigUpdateGroup, groups)
	}
}

// Bus.Close must never reach through to the shared Redis client: the config
// store, the presence watcher and the stats sampler all keep using that same
// client for the life of the process. A Close that called redis.Client.Close
// would leave every one of them on a dead connection.
func TestBusCloseLeavesSharedClientUsable(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	transport := RedisStreamTransport(client, NewSlogAdapter(slog.New(slog.NewTextHandler(io.Discard, nil))))
	bus := newRedisBus(t, transport, client, SourceServer)

	if err := bus.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("shared client unusable after Bus.Close: %v", err)
	}

	// A fresh Bus on the same client still publishes.
	next := newRedisBus(t, transport, client, SourceServer)
	if err := next.Publish(context.Background(), AgentScanResultTopic(), AgentScanResultEventName, "corr-close", []byte(`{}`)); err != nil {
		t.Fatalf("Publish after Close + rebuild: %v", err)
	}
	if got := xlen(t, client, AgentScanResultStream); got != 1 {
		t.Errorf("%s length = %d, want 1", AgentScanResultStream, got)
	}
}

// tunedRedisTransport is RedisStreamTransport with the library's claim/idle
// windows tightened so a reclaim happens in test time rather than the 5s/60s
// defaults. Publish side is the production redisStreamPublisher unchanged.
type tunedRedisTransport struct {
	client redis.UniversalClient
	logger watermill.LoggerAdapter
}

func (t *tunedRedisTransport) Publisher() (StreamPublisher, error) {
	return &redisStreamPublisher{client: t.client}, nil
}

func (t *tunedRedisTransport) Subscriber(group, consumer string) (message.Subscriber, error) {
	return redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:        t.client,
		ConsumerGroup: group,
		Consumer:      consumer,
		Unmarshaller:  minimalUnmarshaller{},
		BlockTime:     10 * time.Millisecond,
		ClaimInterval: 20 * time.Millisecond,
		MaxIdleTime:   40 * time.Millisecond,
	}, t.logger)
}

func TestBusOverRedisReclaimsAfterConsumerDies(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	transport := &tunedRedisTransport{client: client, logger: NewSlogAdapter(slog.New(slog.NewTextHandler(io.Discard, nil)))}
	topic := AgentScanResultTopic()

	// First consumer: picks the message up, signals, then hangs unacked.
	dying := newRedisBus(t, transport, client, SourceServer)
	gotFirst := make(chan struct{}, 1)
	hang := make(chan struct{})
	if err := dying.HandleStream(topic, map[string]StreamHandler{
		AgentScanResultEventName: func(ctx context.Context, _ *Event) error {
			select {
			case gotFirst <- struct{}{}:
			default:
			}
			select {
			case <-hang:
			case <-ctx.Done():
			}
			return ctx.Err()
		},
	}); err != nil {
		t.Fatalf("HandleStream: %v", err)
	}

	dyingCtx, killConsumer := context.WithCancel(context.Background())
	dyingDone := make(chan error, 1)
	go func() { dyingDone <- dying.Run(dyingCtx) }()
	select {
	case <-dying.Ready():
	case <-time.After(2 * time.Second):
		killConsumer()
		t.Fatal("dying bus never became ready")
	}

	// Publish straight to Redis via the production publisher.
	pub := &redisStreamPublisher{client: client}
	envelope := MarshalEventOrFatal(t, newEnvelope(AgentSource("nas-01"), AgentScanResultEventName, "corr-reclaim", []byte(`{}`)))
	if err := pub.Publish(context.Background(), topic.Name, envelope, 1000); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case <-gotFirst:
	case <-time.After(3 * time.Second):
		killConsumer()
		t.Fatal("first consumer never picked up the message")
	}

	// Consumer dies with the message unacked.
	killConsumer()
	close(hang)
	<-dyingDone

	// A fresh consumer in the same group reclaims the idle pending message.
	taker := newRedisBus(t, transport, client, SourceServer)
	reclaimed := make(chan string, 1)
	if err := taker.HandleStream(topic, map[string]StreamHandler{
		AgentScanResultEventName: func(_ context.Context, e *Event) error {
			reclaimed <- e.GetCorrelationId()
			return nil
		},
	}); err != nil {
		t.Fatalf("taker HandleStream: %v", err)
	}
	takerCtx, stopTaker := context.WithCancel(context.Background())
	defer stopTaker()
	takerDone := make(chan error, 1)
	go func() { takerDone <- taker.Run(takerCtx) }()
	select {
	case <-taker.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("taker bus never became ready")
	}

	select {
	case id := <-reclaimed:
		if id != "corr-reclaim" {
			t.Errorf("reclaimed wrong message: %q", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pending message was never reclaimed after its consumer died")
	}

	stopTaker()
	<-takerDone
}
