package busstats

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/redis/go-redis/v9"

	"Metarr/internal/shared/eventbus"
)

func newStreamsSampler(t *testing.T) (*Sampler, *redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return New(client, nil), client, mr
}

func findStream(streams []*StreamStat, name string) *StreamStat {
	for _, s := range streams {
		if s.GetStream() == name {
			return s
		}
	}
	return nil
}

// Every stream in the stream-topic list is a row, including the reserved
// workflow node-result stream, which reads as not-yet-created rather than as
// an error before its listener lands (scphantm/metarr#37).
func TestCollectStreamsListsEveryStreamTopic(t *testing.T) {
	sampler, _, _ := newStreamsSampler(t)

	streams, _, _ := sampler.collectStreams(context.Background())

	for _, want := range []string{
		eventbus.AgentScanResultStream,
		eventbus.AgentNodeResultStream,
	} {
		if findStream(streams, want) == nil {
			t.Errorf("stream %q is not in the collected list", want)
		}
	}

	node := findStream(streams, eventbus.AgentNodeResultStream)
	if node == nil {
		t.Fatalf("%s missing from the stream list", eventbus.AgentNodeResultStream)
	}
	if node.GetExists() {
		t.Errorf("%s should read as not-yet-created before its listener lands", eventbus.AgentNodeResultStream)
	}
	if node.GetError() != "" {
		t.Errorf("a reserved stream with no consumer group must not carry an error, got %q", node.GetError())
	}
}

// A per-agent command stream that exists in Redis is discovered by glob and
// labelled with the consumer group that reads it.
func TestCollectStreamsDiscoversPerAgentCommandStream(t *testing.T) {
	sampler, client, _ := newStreamsSampler(t)
	ctx := context.Background()

	stream := eventbus.AgentCommandStream("nas-01")
	if err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"payload": "{}"},
	}).Err(); err != nil {
		t.Fatalf("seed per-agent stream: %v", err)
	}

	streams, _, _ := sampler.collectStreams(ctx)
	found := findStream(streams, stream)
	if found == nil {
		t.Fatalf("per-agent command stream %s was not discovered", stream)
	}
	if !found.GetExists() {
		t.Errorf("a seeded stream should read as existing")
	}
	if got := found.GetLength(); got != 1 {
		t.Errorf("length = %d, want 1", got)
	}
}

// A seeded stream with a consumer group and unacknowledged entries produces
// a group row carrying consumer count, pending, lag and the age of the
// oldest pending entry.
func TestCollectStreamGroupRows(t *testing.T) {
	sampler, client, _ := newStreamsSampler(t)
	ctx := context.Background()

	stream := eventbus.AgentScanResultStream
	group := eventbus.AgentScanResultGroup

	// One entry timestamped ~90s ago so its pending age is a real number.
	oldMillis := time.Now().Add(-90 * time.Second).UnixMilli()
	if err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		ID:     strconv.FormatInt(oldMillis, 10) + "-0",
		Values: map[string]any{"payload": "{}"},
	}).Err(); err != nil {
		t.Fatalf("seed stream entry: %v", err)
	}
	if err := client.XGroupCreate(ctx, stream, group, "0").Err(); err != nil {
		t.Fatalf("create group: %v", err)
	}
	// Read the entry into the group without acking it, so it stays pending.
	if err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "metarr-server",
		Streams:  []string{stream, ">"},
	}).Err(); err != nil {
		t.Fatalf("read into group: %v", err)
	}

	streams, _, _ := sampler.collectStreams(ctx)
	found := findStream(streams, stream)
	if found == nil {
		t.Fatalf("stream %s missing", stream)
	}
	if !found.GetExists() {
		t.Fatalf("stream should exist once a group has been created")
	}
	if len(found.GetGroups()) != 1 {
		t.Fatalf("want exactly one group row, got %d", len(found.GetGroups()))
	}

	g := found.GetGroups()[0]
	if g.GetName() != group {
		t.Errorf("group name = %q, want %q", g.GetName(), group)
	}
	if g.GetConsumers() != 1 {
		t.Errorf("consumers = %d, want 1", g.GetConsumers())
	}
	if g.GetPending() != 1 {
		t.Errorf("pending = %d, want 1", g.GetPending())
	}
	if g.GetLag() < 0 {
		t.Errorf("lag = %d, want a non-negative count", g.GetLag())
	}
	if age := g.GetOldestPendingAgeSeconds(); age < 60 || age > 600 {
		t.Errorf("oldest_pending_age_seconds = %d, want roughly 90", age)
	}
}

// A group with nothing pending reports a zero oldest-pending age rather than
// a bogus one derived from an empty XPENDING summary.
func TestCollectStreamGroupWithNoPendingHasZeroAge(t *testing.T) {
	sampler, client, _ := newStreamsSampler(t)
	ctx := context.Background()

	stream := eventbus.AgentScanResultStream
	group := eventbus.AgentScanResultGroup
	if err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"payload": "{}"},
	}).Err(); err != nil {
		t.Fatalf("seed stream entry: %v", err)
	}
	if err := client.XGroupCreate(ctx, stream, group, "$").Err(); err != nil {
		t.Fatalf("create group: %v", err)
	}

	streams, _, _ := sampler.collectStreams(ctx)
	found := findStream(streams, stream)
	if found == nil || len(found.GetGroups()) != 1 {
		t.Fatalf("expected one group row, got %+v", found)
	}
	if age := found.GetGroups()[0].GetOldestPendingAgeSeconds(); age != 0 {
		t.Errorf("oldest_pending_age_seconds = %d, want 0 for an idle group", age)
	}
}

// An unreadable stream carries a per-stream error and does not cost the
// caller the rest of the snapshot: the other stream rows and the server
// counters still come back intact.
func TestPassKeepsSnapshotWhenOneStreamIsUnreadable(t *testing.T) {
	sampler, _, mr := newStreamsSampler(t)
	ctx := context.Background()

	// Fail XINFO STREAM for the one stream, leaving every other Redis call
	// working. The failure is not a "no such key", so the row must carry the
	// error rather than read as not-yet-created.
	mr.Server().SetPreHook(func(c *server.Peer, cmd string, args ...string) bool {
		if strings.EqualFold(cmd, "XINFO") && len(args) >= 2 &&
			strings.EqualFold(args[0], "STREAM") && args[1] == eventbus.AgentScanResultStream {
			c.WriteError("ERR simulated stream read failure")
			return true
		}
		return false
	})

	sampler.pass(ctx)
	snap := sampler.Get()
	if snap == nil {
		t.Fatal("pass produced no snapshot")
	}

	bad := findStream(snap.GetStreams(), eventbus.AgentScanResultStream)
	if bad == nil || bad.GetError() == "" {
		t.Fatalf("the unreadable stream should carry an error, got %+v", bad)
	}

	if good := findStream(snap.GetStreams(), eventbus.AgentNodeResultStream); good == nil || good.GetError() != "" {
		t.Errorf("a healthy stream row was lost or flagged: %+v", good)
	}
	if snap.GetServer() == nil || len(snap.GetServer().GetFieldErrors()) != 0 {
		t.Errorf("server counters should be intact, got %+v", snap.GetServer())
	}
}
