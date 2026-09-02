package eventbus

import (
	"context"
	"testing"
	"time"
)

// InMemoryPubSub is the test adapter the Bus's Pub/Sub half runs on. These
// pin the three properties the Bus depends on, independently of a running
// Bus: a subscription is live the instant Subscribe returns, Publish fans one
// payload out to every subscriber on the channel, and Close ends the stream.

func TestInMemoryPubSubSubscriptionIsLiveOnReturn(t *testing.T) {
	broker := InMemoryPubSub()
	ctx := context.Background()

	sub, err := broker.Subscribe(ctx, "c1")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })
	if err := sub.Receive(ctx); err != nil {
		t.Fatalf("Receive (the SUBSCRIBE ack): %v", err)
	}

	// A publish that follows the ack must be delivered — this is the ordering
	// Bus.Request leans on when it subscribes to the reply channel before
	// publishing the request.
	if err := broker.Publish(ctx, "c1", []byte("hello")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case got := <-sub.Channel():
		if string(got) != "hello" {
			t.Errorf("got %q, want hello", got)
		}
	case <-time.After(time.Second):
		t.Fatal("payload published after the ack was not delivered")
	}
}

func TestInMemoryPubSubFansOutToEverySubscriber(t *testing.T) {
	broker := InMemoryPubSub()
	ctx := context.Background()

	subs := make([]PubSubSubscription, 3)
	for i := range subs {
		s, err := broker.Subscribe(ctx, "fan")
		if err != nil {
			t.Fatalf("Subscribe %d: %v", i, err)
		}
		subs[i] = s
		t.Cleanup(func() { _ = s.Close() })
	}

	if err := broker.Publish(ctx, "fan", []byte("x")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	for i, s := range subs {
		select {
		case got := <-s.Channel():
			if string(got) != "x" {
				t.Errorf("sub %d got %q, want x", i, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("sub %d never received the fan-out payload", i)
		}
	}
}

func TestInMemoryPubSubCloseEndsTheChannelAndDropsFuturePublishes(t *testing.T) {
	broker := InMemoryPubSub()
	ctx := context.Background()

	sub, err := broker.Subscribe(ctx, "c")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := sub.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case _, ok := <-sub.Channel():
		if ok {
			t.Fatal("Channel delivered after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("Channel was not closed by Close")
	}

	// Publishing to a channel whose only subscriber has gone is a no-op, not
	// an error and not a panic on a closed channel.
	if err := broker.Publish(ctx, "c", []byte("late")); err != nil {
		t.Fatalf("Publish after Close: %v", err)
	}
	// Close is idempotent.
	if err := sub.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
