package eventbus

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func purgeTestClient(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

// seedStreamEntry adds one entry timestamped `age` in the past so a MINID
// trim at now is guaranteed to drop it.
func seedStreamEntry(t *testing.T, client *redis.Client, stream string, age time.Duration) {
	t.Helper()
	id := strconv.FormatInt(time.Now().Add(-age).UnixMilli(), 10) + "-0"
	if err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream,
		ID:     id,
		Values: map[string]any{"payload": "{}"},
	}).Err(); err != nil {
		t.Fatalf("seed %s: %v", stream, err)
	}
}

// A stream that has never been created is trimmed to a no-op and is not an
// error — a purge-all batch must not break on the reserved node-result
// stream.
func TestPurgeStreamOnUncreatedStream(t *testing.T) {
	client, _ := purgeTestClient(t)

	result, err := PurgeStream(context.Background(), client, AgentNodeResultStream)
	if err != nil {
		t.Fatalf("PurgeStream on an uncreated stream: %v", err)
	}
	if result.Dropped != 0 {
		t.Errorf("dropped = %d, want 0", result.Dropped)
	}
	if len(result.GroupsFastForwarded) != 0 {
		t.Errorf("GroupsFastForwarded = %v, want none", result.GroupsFastForwarded)
	}
}

// A stream with entries but no consumer group is trimmed only, not an error.
func TestPurgeStreamWithNoGroups(t *testing.T) {
	client, _ := purgeTestClient(t)
	ctx := context.Background()

	seedStreamEntry(t, client, AgentScanResultStream, 30*time.Second)
	seedStreamEntry(t, client, AgentScanResultStream, 20*time.Second)

	result, err := PurgeStream(ctx, client, AgentScanResultStream)
	if err != nil {
		t.Fatalf("PurgeStream: %v", err)
	}
	if result.Dropped != 2 {
		t.Errorf("dropped = %d, want 2", result.Dropped)
	}
	if len(result.GroupsFastForwarded) != 0 {
		t.Errorf("GroupsFastForwarded = %v, want none", result.GroupsFastForwarded)
	}
}

// PurgeAllStreams clears every stream DiscoverStreamTopics resolves: the
// static rows, a discovered per-agent command stream, and a reserved stream
// with no consumer group.
func TestPurgeAllStreams(t *testing.T) {
	client, _ := purgeTestClient(t)
	ctx := context.Background()

	agentStream := AgentCommandStream("nas-01")
	seedStreamEntry(t, client, AgentScanResultStream, 40*time.Second)
	seedStreamEntry(t, client, AgentScanResultStream, 30*time.Second)
	seedStreamEntry(t, client, agentStream, 25*time.Second)
	// AgentNodeResultStream is never created.

	results, err := PurgeAllStreams(ctx, client)
	if err != nil {
		t.Fatalf("PurgeAllStreams: %v", err)
	}

	dropped := map[string]int64{}
	for _, r := range results {
		dropped[r.Stream] = r.Dropped
	}
	for _, want := range []string{
		AgentScanResultStream, AgentNodeResultStream, agentStream,
	} {
		if _, ok := dropped[want]; !ok {
			t.Errorf("stream %q was not purged", want)
		}
	}
	if dropped[AgentScanResultStream] != 2 {
		t.Errorf("%s dropped = %d, want 2", AgentScanResultStream, dropped[AgentScanResultStream])
	}
	if dropped[agentStream] != 1 {
		t.Errorf("%s dropped = %d, want 1", agentStream, dropped[agentStream])
	}

	if n, _ := client.XLen(ctx, AgentScanResultStream).Result(); n != 0 {
		t.Errorf("%s length after purge-all = %d, want 0", AgentScanResultStream, n)
	}
}
