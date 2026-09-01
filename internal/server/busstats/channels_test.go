package busstats

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"Metarr/internal/shared/eventbus"
)

// subscribe opens n independent subscriber connections to channel and blocks
// until every one has had its subscribe confirmed, so a following PUBSUB
// NUMSUB sees them all. Each connection is closed when the test ends.
func subscribe(t *testing.T, addr, channel string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		client := redis.NewClient(&redis.Options{Addr: addr})
		t.Cleanup(func() { _ = client.Close() })

		sub := client.Subscribe(context.Background(), channel)
		t.Cleanup(func() { _ = sub.Close() })

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if _, err := sub.Receive(ctx); err != nil {
			cancel()
			t.Fatalf("subscribe to %s: %v", channel, err)
		}
		cancel()
	}
}

// Every declared channel appears as a row even with nothing subscribed, and a
// dead declared channel is a zero row flagged as known rather than an absent
// one.
func TestCollectChannelsIncludesEveryKnownChannelAtZero(t *testing.T) {
	sampler, _ := newTestSampler(t)

	sampler.pass(context.Background())

	byName := map[string]*ChannelStat{}
	for _, ch := range sampler.Get().GetChannels() {
		byName[ch.GetChannel()] = ch
	}

	for _, name := range eventbus.KnownPubSubChannels() {
		row, ok := byName[name]
		if !ok {
			t.Fatalf("known channel %q is missing from the snapshot", name)
		}
		if !row.GetKnown() {
			t.Errorf("known channel %q is not flagged known", name)
		}
		if row.GetSubscribers() != 0 {
			t.Errorf("channel %q: subscribers = %d, want 0 (nothing subscribed)", name, row.GetSubscribers())
		}
	}
}

// Subscriber counts on the rows match what PUBSUB NUMSUB reports for the
// seeded subscribers.
func TestCollectChannelsCountsSeededSubscribers(t *testing.T) {
	sampler, mr := newTestSampler(t)

	subscribe(t, mr.Addr(), eventbus.HeartbeatRequestChannel, 3)
	subscribe(t, mr.Addr(), eventbus.LogChannel, 1)

	sampler.pass(context.Background())

	counts := map[string]int64{}
	for _, ch := range sampler.Get().GetChannels() {
		counts[ch.GetChannel()] = ch.GetSubscribers()
	}

	if got := counts[eventbus.HeartbeatRequestChannel]; got != 3 {
		t.Errorf("%s subscribers = %d, want 3", eventbus.HeartbeatRequestChannel, got)
	}
	if got := counts[eventbus.LogChannel]; got != 1 {
		t.Errorf("%s subscribers = %d, want 1", eventbus.LogChannel, got)
	}
}

// A channel discovered live against Redis that is not on the declared list —
// the per-correlation-id reply channels — is carried as a row flagged as not
// known, so the dashboard can show it as transient.
func TestCollectChannelsMarksDiscoveredChannelsTransient(t *testing.T) {
	sampler, mr := newTestSampler(t)

	transient := eventbus.ReplyChannel("corr-1234")
	subscribe(t, mr.Addr(), transient, 1)

	sampler.pass(context.Background())

	var row *ChannelStat
	for _, ch := range sampler.Get().GetChannels() {
		if ch.GetChannel() == transient {
			row = ch
		}
	}
	if row == nil {
		t.Fatalf("live channel %q is missing from the snapshot", transient)
	}
	if row.GetKnown() {
		t.Errorf("discovered channel %q should not be flagged known", transient)
	}
	if row.GetSubscribers() != 1 {
		t.Errorf("%s subscribers = %d, want 1", transient, row.GetSubscribers())
	}
}

// Declared channels sort ahead of discovered ones, and the order is otherwise
// alphabetical, so the table does not reshuffle between passes.
func TestCollectChannelsOrderIsStable(t *testing.T) {
	sampler, mr := newTestSampler(t)

	subscribe(t, mr.Addr(), eventbus.ReplyChannel("aaa"), 1)
	subscribe(t, mr.Addr(), eventbus.ReplyChannel("zzz"), 1)

	sampler.pass(context.Background())

	channels := sampler.Get().GetChannels()
	firstDiscovered := -1
	for i, ch := range channels {
		if !ch.GetKnown() && firstDiscovered == -1 {
			firstDiscovered = i
		}
		if ch.GetKnown() && firstDiscovered != -1 {
			t.Fatalf("declared channel %q sorted after a discovered one", ch.GetChannel())
		}
	}
}
