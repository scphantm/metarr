package eventbus

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// The one Redis-Streams-specific test (docs/adr/0006, testing seam 2): a
// miniredis instead of a real Redis process, covering the two behaviours the
// library owns rather than the Router — the consumer group is created on
// first subscribe, and a message whose consumer died between read and ack is
// reclaimed by XAUTOCLAIM and processed by another consumer in the group.
func TestRouterOverRedisCreatesGroupAndReclaimsAfterConsumerDies(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	logger := NewSlogAdapter(slog.New(slog.NewTextHandler(io.Discard, nil)))

	publisher, err := redisstream.NewPublisher(redisstream.PublisherConfig{Client: client}, logger)
	if err != nil {
		t.Fatalf("publisher: %v", err)
	}
	t.Cleanup(func() { _ = publisher.Close() })

	const (
		stream = "events.test_reclaim"
		group  = "test_reclaim_group"
	)

	// Tight claim/idle windows so the reclaim happens in test time rather
	// than the library's 5s/60s defaults.
	newSub := func(consumer string) SubscriberFactory {
		return func(_, _ string) (message.Subscriber, error) {
			return redisstream.NewSubscriber(redisstream.SubscriberConfig{
				Client:        client,
				ConsumerGroup: group,
				Consumer:      consumer,
				BlockTime:     10 * time.Millisecond,
				ClaimInterval: 20 * time.Millisecond,
				MaxIdleTime:   40 * time.Millisecond,
			}, logger)
		}
	}

	// Consumer "dies-mid-handler": it receives the message, signals, and then
	// hangs without acking. Cancelling its context stops the router with the
	// message still pending in the group.
	gotFirst := make(chan struct{}, 1)
	hang := make(chan struct{})
	dyingRouter, err := NewRouter(publisher, newSub("dies-mid-handler"), testPolicy(), logger)
	if err != nil {
		t.Fatalf("dying router: %v", err)
	}
	if err := dyingRouter.Handle("reclaim", stream, group, "dies-mid-handler",
		func(ctx context.Context, _ *Event) error {
			select {
			case gotFirst <- struct{}{}:
			default:
			}
			select {
			case <-hang:
			case <-ctx.Done():
			}
			return ctx.Err()
		}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	dyingCtx, killConsumer := context.WithCancel(context.Background())
	dyingDone := make(chan struct{})
	go func() { _ = dyingRouter.Run(dyingCtx); close(dyingDone) }()
	<-dyingRouter.Running()

	publishEnvelope(t, publisher, stream,
		NewEvent(SourceServer, "test.reclaim", "corr-reclaim", []byte(`{}`)))

	receiveSignal(t, gotFirst, 2*time.Second, "the first consumer to pick up the message")

	// The library created the group on first subscribe.
	groups, err := client.XInfoGroups(context.Background(), stream).Result()
	if err != nil {
		t.Fatalf("XInfoGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != group {
		t.Fatalf("expected consumer group %q to have been created, got %+v", group, groups)
	}

	// The consumer dies with the message unacked.
	killConsumer()
	close(hang)
	<-dyingDone

	// A fresh consumer in the same group reclaims the idle pending message.
	reclaimed := make(chan string, 1)
	liveRouter, err := NewRouter(publisher, newSub("takes-over"), testPolicy(), logger)
	if err != nil {
		t.Fatalf("live router: %v", err)
	}
	if err := liveRouter.Handle("reclaim", stream, group, "takes-over",
		func(_ context.Context, event *Event) error {
			reclaimed <- event.GetCorrelationId()
			return nil
		}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	liveCtx, stopLive := context.WithCancel(context.Background())
	defer stopLive()
	go func() { _ = liveRouter.Run(liveCtx) }()
	<-liveRouter.Running()

	select {
	case id := <-reclaimed:
		if id != "corr-reclaim" {
			t.Errorf("reclaimed the wrong message: %q", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the pending message was never reclaimed after its consumer died")
	}

	// The reclaiming consumer acks after its handler returns; watermill does
	// that on its own goroutine, so give the pending entry a moment to drain.
	deadline := time.Now().Add(2 * time.Second)
	for {
		pending, err := client.XPending(context.Background(), stream, group).Result()
		if err != nil {
			t.Fatalf("XPending: %v", err)
		}
		if pending.Count == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Errorf("expected no pending messages after reclaim and ack, got %d", pending.Count)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func receiveSignal(t *testing.T, ch <-chan struct{}, d time.Duration, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(d):
		t.Fatalf("timed out waiting for %s", what)
	}
}
