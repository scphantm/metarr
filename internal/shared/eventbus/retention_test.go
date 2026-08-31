package eventbus

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func discardSlog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newRetentionRedis(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return server, client
}

// A stream held at its cap under sustained publishing (acceptance criterion).
func TestStreamBusCapsEveryPublishByTier(t *testing.T) {
	_, client := newRetentionRedis(t)

	policy := RetentionPolicy{MaxLenHigh: 10, MaxLenDefault: 4, RetentionHours: 48}
	bus, err := NewStreamBus(client, policy, NewSlogAdapter(discardSlog()))
	if err != nil {
		t.Fatalf("NewStreamBus: %v", err)
	}

	ctx := context.Background()
	for i := range 50 {
		id := strconv.Itoa(i)
		if err := bus.Fire(ctx, SystemConfigUpdateStream, NewEvent(SourceServer, SystemConfigUpdateEventName, id, []byte(`{}`))); err != nil {
			t.Fatalf("fire default-tier: %v", err)
		}
		if err := bus.Fire(ctx, AgentScanResultStream, NewEvent(AgentSource("nas-01"), AgentScanResultEventName, id, []byte(`{}`))); err != nil {
			t.Fatalf("fire high-tier: %v", err)
		}
	}

	if got := xlen(t, client, SystemConfigUpdateStream); got > policy.MaxLenDefault {
		t.Errorf("default-tier stream length %d exceeds cap %d", got, policy.MaxLenDefault)
	}
	if got := xlen(t, client, AgentScanResultStream); got > policy.MaxLenHigh {
		t.Errorf("high-tier stream length %d exceeds cap %d", got, policy.MaxLenHigh)
	}
	// The high-tier cap really is higher than the default one.
	if xlen(t, client, AgentScanResultStream) <= xlen(t, client, SystemConfigUpdateStream) {
		t.Error("expected the high-volume stream to retain more entries than the default-tier one")
	}
}

// The sweep removes entries older than the retention window and keeps the
// rest, across fixed streams and discovered per-agent command streams.
func TestRetentionSweepTrimsByAge(t *testing.T) {
	_, client := newRetentionRedis(t)
	ctx := context.Background()

	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	policy := RetentionPolicy{MaxLenHigh: 1_000, MaxLenDefault: 1_000, RetentionHours: 48}

	agentStream := AgentCommandStream("nas-01")
	streams := []string{SystemConfigUpdateStream, AgentScanResultStream, DeadLetterStream, agentStream}

	for _, stream := range streams {
		// One entry three days old, one a minute old.
		addAt(t, client, stream, now.Add(-72*time.Hour), "old")
		addAt(t, client, stream, now.Add(-time.Minute), "fresh")
	}

	sweeper := NewRetentionSweeper(client, policy, discardSlog())
	sweeper.now = func() time.Time { return now }

	if err := sweeper.SweepOnce(ctx); err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}

	for _, stream := range streams {
		entries, err := client.XRange(ctx, stream, "-", "+").Result()
		if err != nil {
			t.Fatalf("XRange %s: %v", stream, err)
		}
		if len(entries) != 1 {
			t.Fatalf("%s: expected only the fresh entry to survive, got %d entries", stream, len(entries))
		}
		if entries[0].Values["marker"] != "fresh" {
			t.Errorf("%s: survivor is %v, want the fresh entry", stream, entries[0].Values)
		}
	}
}

func TestRetentionSweepIgnoresAStreamThatDoesNotExist(t *testing.T) {
	_, client := newRetentionRedis(t)

	sweeper := NewRetentionSweeper(client, DefaultRetentionPolicy(), discardSlog())
	if err := sweeper.SweepOnce(context.Background()); err != nil {
		t.Fatalf("a sweep over streams that were never created is not an error: %v", err)
	}
}

func TestDefaultRetentionPolicyMatchesTheDocumentedFloor(t *testing.T) {
	policy := DefaultRetentionPolicy()
	if policy.RetentionHours != 48 {
		t.Errorf("RetentionHours = %d, want the documented 48-hour floor", policy.RetentionHours)
	}
	if policy.MaxLenHigh <= policy.MaxLenDefault {
		t.Errorf("MaxLenHigh (%d) should exceed MaxLenDefault (%d)", policy.MaxLenHigh, policy.MaxLenDefault)
	}
	caps := policy.Maxlens()
	for _, stream := range HighVolumeStreams() {
		if caps[stream] != policy.MaxLenHigh {
			t.Errorf("Maxlens()[%q] = %d, want MaxLenHigh %d", stream, caps[stream], policy.MaxLenHigh)
		}
	}
}

func xlen(t *testing.T, client redis.UniversalClient, stream string) int64 {
	t.Helper()
	n, err := client.XLen(context.Background(), stream).Result()
	if err != nil {
		t.Fatalf("XLen %s: %v", stream, err)
	}
	return n
}

// addAt appends an entry to stream with a Redis Stream ID whose millisecond
// component is at, so age-based trimming has something deterministic to act
// on.
func addAt(t *testing.T, client redis.UniversalClient, stream string, at time.Time, marker string) {
	t.Helper()
	id := strconv.FormatInt(at.UnixMilli(), 10) + "-0"
	if err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream,
		ID:     id,
		Values: map[string]any{"marker": marker},
	}).Err(); err != nil {
		t.Fatalf("XAdd %s @ %s: %v", stream, id, err)
	}
}
