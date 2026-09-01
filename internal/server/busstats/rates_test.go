package busstats

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"Metarr/internal/shared/eventbus"
)

func newRatesSampler(t *testing.T, history int) (*Sampler, *redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return New(client, nil, WithHistory(history)), client, mr
}

// Under plain miniredis — which reports neither a stream's entries-added nor a
// group's entries-read — the rate fields are present and sit at a sane zero
// rather than spiking or erroring, and every numeric stream/group metric still
// carries a series that grows one sample per pass. The exact-value delta is
// covered by the real-Redis test in rates_realredis_test.go.
func TestRateShapeUnderMiniredis(t *testing.T) {
	sampler, client, _ := newRatesSampler(t, 8)
	ctx := context.Background()

	stream := eventbus.AgentScanResultStream
	group := eventbus.AgentScanResultGroup
	if err := client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{"payload": "{}"}}).Err(); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	if err := client.XGroupCreate(ctx, stream, group, "0").Err(); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := client.XReadGroup(ctx, &redis.XReadGroupArgs{Group: group, Consumer: "metarr-server", Streams: []string{stream, ">"}}).Err(); err != nil {
		t.Fatalf("read into group: %v", err)
	}

	sampler.pass(ctx)
	sampler.pass(ctx)

	st := findStream(sampler.Get().GetStreams(), stream)
	if st == nil {
		t.Fatal("stream row missing")
	}
	if st.GetError() != "" {
		t.Fatalf("stream carried an error: %q", st.GetError())
	}
	if st.GetPublishRate() != 0 {
		t.Errorf("publish_rate = %d, want 0 under miniredis", st.GetPublishRate())
	}
	if len(st.GetLengthSeries()) != 2 || len(st.GetPublishRateSeries()) != 2 {
		t.Errorf("stream series did not accumulate one sample per pass: length=%d publish=%d",
			len(st.GetLengthSeries()), len(st.GetPublishRateSeries()))
	}
	if len(st.GetGroups()) != 1 {
		t.Fatalf("want one group row, got %d", len(st.GetGroups()))
	}
	g := st.GetGroups()[0]
	if g.GetConsumeRate() != 0 {
		t.Errorf("consume_rate = %d, want 0 under miniredis", g.GetConsumeRate())
	}
	for name, series := range map[string][]int64{
		"consumers":      g.GetConsumersSeries(),
		"pending":        g.GetPendingSeries(),
		"lag":            g.GetLagSeries(),
		"oldest_pending": g.GetOldestPendingAgeSecondsSeries(),
		"consume_rate":   g.GetConsumeRateSeries(),
	} {
		if len(series) != 2 {
			t.Errorf("group %s series length = %d, want 2", name, len(series))
		}
	}
}

// Every stream and group series is a ring buffer capped at the configured
// window: more passes than the window leaves each series exactly window-long.
func TestStreamAndGroupSeriesCapAtWindow(t *testing.T) {
	const window = 4
	sampler, client, _ := newRatesSampler(t, window)
	ctx := context.Background()

	stream := eventbus.AgentScanResultStream
	group := eventbus.AgentScanResultGroup
	if err := client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{"payload": "{}"}}).Err(); err != nil {
		t.Fatalf("seed stream: %v", err)
	}
	if err := client.XGroupCreate(ctx, stream, group, "0").Err(); err != nil {
		t.Fatalf("create group: %v", err)
	}

	for i := 0; i < window+5; i++ {
		sampler.pass(ctx)
	}

	st := findStream(sampler.Get().GetStreams(), stream)
	if got := len(st.GetLengthSeries()); got != window {
		t.Errorf("length_series length = %d, want %d (capped)", got, window)
	}
	if got := len(st.GetPublishRateSeries()); got != window {
		t.Errorf("publish_rate_series length = %d, want %d (capped)", got, window)
	}
	g := st.GetGroups()[0]
	if got := len(g.GetPendingSeries()); got != window {
		t.Errorf("pending_series length = %d, want %d (capped)", got, window)
	}
	if got := len(g.GetConsumeRateSeries()); got != window {
		t.Errorf("consume_rate_series length = %d, want %d (capped)", got, window)
	}
}

// History and previous-counter state for a stream that drops out of the
// topology are discarded rather than growing without bound.
func TestVanishedStreamHistoryIsDropped(t *testing.T) {
	sampler, client, _ := newRatesSampler(t, 8)
	ctx := context.Background()

	// A discovered per-agent command stream: present only while its key is.
	stream := eventbus.AgentCommandStream("nas-01")
	if err := client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{"payload": "{}"}}).Err(); err != nil {
		t.Fatalf("seed stream: %v", err)
	}

	sampler.pass(ctx)
	if _, ok := sampler.streamHist[stream]; !ok {
		t.Fatalf("expected history for the discovered stream after a pass")
	}

	if err := client.Del(ctx, stream).Err(); err != nil {
		t.Fatalf("delete stream: %v", err)
	}
	sampler.pass(ctx)

	if _, ok := sampler.streamHist[stream]; ok {
		t.Errorf("history for a vanished stream was not dropped")
	}
}

func TestNonNegativeDelta(t *testing.T) {
	cases := []struct {
		prev, cur, want int64
	}{
		{0, 0, 0},
		{10, 13, 3},
		{13, 13, 0},
		{100, 5, 0}, // counter reset — no negative spike
	}
	for _, c := range cases {
		if got := nonNegativeDelta(c.prev, c.cur); got != c.want {
			t.Errorf("nonNegativeDelta(%d, %d) = %d, want %d", c.prev, c.cur, got, c.want)
		}
	}
}
