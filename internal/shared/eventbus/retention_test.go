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

// Every stream is held at the one cap under sustained publishing (acceptance
// criterion): there is no tier, so the same MAXLEN — read from the late-bound
// policy on every Bus.Publish — applies to all of them.
func TestBusCapsEveryPublish(t *testing.T) {
	_, client := newRetentionRedis(t)

	const maxLen int64 = 4
	policy := testBusPolicy()
	policy.Retention.MaxLen = maxLen
	bus, err := New(Config{
		Redis:   client,
		Source:  SourceServer,
		Streams: RedisStreamTransport(client, NewSlogAdapter(discardSlog())),
		Policy:  func() BusPolicy { return policy },
		Logger:  discardSlog(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	agentStream := AgentCommandTopic("nas-01")
	for i := range 50 {
		id := strconv.Itoa(i)
		if err := bus.Publish(ctx, agentStream, AgentScanCommandEventName, id, []byte(`{}`)); err != nil {
			t.Fatalf("publish agent command: %v", err)
		}
		if err := bus.Publish(ctx, AgentScanResultTopic(), AgentScanResultEventName, id, []byte(`{}`)); err != nil {
			t.Fatalf("publish agent_scan_results: %v", err)
		}
	}

	for _, stream := range []string{AgentCommandStream("nas-01"), AgentScanResultStream} {
		if got := xlen(t, client, stream); got > maxLen {
			t.Errorf("%s length %d exceeds the one cap %d", stream, got, maxLen)
		}
	}
}

// The sweep removes entries older than the retention window and keeps the
// rest, across fixed streams and discovered per-agent command streams.
func TestRetentionSweepTrimsByAge(t *testing.T) {
	_, client := newRetentionRedis(t)
	ctx := context.Background()

	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	policy := RetentionPolicy{MaxLen: 1_000, RetentionHours: 48}

	agentStream := AgentCommandStream("nas-01")
	streams := []string{AgentScanResultStream, AgentNodeResultStream, agentStream}

	for _, stream := range streams {
		// One entry three days old, one a minute old.
		addAt(t, client, stream, now.Add(-72*time.Hour), "old")
		addAt(t, client, stream, now.Add(-time.Minute), "fresh")
	}

	sweeper := NewRetentionSweeper(client, policy, DefaultSweepInterval, discardSlog())
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

	sweeper := NewRetentionSweeper(client, DefaultBusPolicy().Retention, DefaultSweepInterval, discardSlog())
	if err := sweeper.SweepOnce(context.Background()); err != nil {
		t.Fatalf("a sweep over streams that were never created is not an error: %v", err)
	}
}

func TestDefaultRetentionPolicyMatchesTheDocumentedFloor(t *testing.T) {
	policy := DefaultBusPolicy().Retention
	if policy.RetentionHours != 48 {
		t.Errorf("RetentionHours = %d, want the documented 48-hour floor", policy.RetentionHours)
	}
	if policy.MaxLen <= 0 {
		t.Errorf("MaxLen = %d, want a positive cap", policy.MaxLen)
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

// addAt appends an entry to stream with a Redis Stream ID timestamped at, so
// age-based trimming has something deterministic to act on.
func addAt(t *testing.T, client redis.UniversalClient, stream string, at time.Time, marker string) {
	t.Helper()
	id := StreamIDForTime(at)
	if err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream,
		ID:     id,
		Values: map[string]any{"marker": marker},
	}).Err(); err != nil {
		t.Fatalf("XAdd %s @ %s: %v", stream, id, err)
	}
}
