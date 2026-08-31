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
// criterion): there is no tier, so the same MAXLEN applies to all of them.
func TestStreamBusCapsEveryPublish(t *testing.T) {
	_, client := newRetentionRedis(t)

	policy := RetentionPolicy{MaxLen: 4, RetentionHours: 48}
	bus, err := NewStreamBus(client, policy, NewSlogAdapter(discardSlog()))
	if err != nil {
		t.Fatalf("NewStreamBus: %v", err)
	}

	ctx := context.Background()
	for i := range 50 {
		id := strconv.Itoa(i)
		if err := bus.Publish(ctx, SystemConfigUpdateTopic(), NewEvent(SourceServer, SystemConfigUpdateEventName, id, []byte(`{}`))); err != nil {
			t.Fatalf("publish system_config_update: %v", err)
		}
		if err := bus.Publish(ctx, AgentScanResultTopic(), NewEvent(AgentSource("nas-01"), AgentScanResultEventName, id, []byte(`{}`))); err != nil {
			t.Fatalf("publish agent_scan_results: %v", err)
		}
	}

	for _, stream := range []string{SystemConfigUpdateStream, AgentScanResultStream} {
		if got := xlen(t, client, stream); got > policy.MaxLen {
			t.Errorf("%s length %d exceeds the one cap %d", stream, got, policy.MaxLen)
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
	streams := []string{SystemConfigUpdateStream, AgentScanResultStream, AgentNodeResultStream, agentStream}

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
