//go:build realredis

// These tests need a real Redis: miniredis does not implement XGROUP SETID,
// so the consumer-group fast-forward half of a purge can only be exercised
// against the real server. Run them with:
//
//	go test -tags realredis ./internal/shared/eventbus/...
//
// pointing REDIS_ADDR / REDIS_PASSWORD at a throwaway instance (defaults
// 127.0.0.1:6379 and the docker-compose dev password "metarr", database 15,
// which the test flushes).
package eventbus

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func realRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	password, ok := os.LookupEnv("REDIS_PASSWORD")
	if !ok {
		password = "metarr" // docker-compose.yml default
	}
	client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: 15})
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Skipf("no real Redis at %s: %v", addr, err)
	}
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush test db: %v", err)
	}
	t.Cleanup(func() {
		_ = client.FlushDB(ctx)
		_ = client.Close()
	})
	return client
}

func seedPastEntry(t *testing.T, client *redis.Client, stream string, age time.Duration) {
	t.Helper()
	id := strconv.FormatInt(time.Now().Add(-age).UnixMilli(), 10) + "-0"
	if err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream, ID: id, Values: map[string]any{"payload": "{}"},
	}).Err(); err != nil {
		t.Fatalf("seed %s: %v", stream, err)
	}
}

// Purge on a seeded stream whose group has pending entries: the stream goes
// to zero length, the group survives fast-forwarded to the tail with its
// last-delivered-id at $, and its pending list is cleared.
func TestPurgeStreamFastForwardsGroupOnRealRedis(t *testing.T) {
	client := realRedisClient(t)
	ctx := context.Background()

	stream := SystemConfigUpdateStream
	group := SystemConfigUpdateGroup

	for i := 0; i < 5; i++ {
		seedPastEntry(t, client, stream, time.Duration(300-i*10)*time.Second)
	}
	if err := client.XGroupCreate(ctx, stream, group, "0").Err(); err != nil {
		t.Fatalf("create group: %v", err)
	}
	// Read all five into the group without acking, so they stay pending.
	if err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: group, Consumer: "metarr-server", Streams: []string{stream, ">"},
	}).Err(); err != nil {
		t.Fatalf("read into group: %v", err)
	}

	// The tail entry the group should sit behind after the purge.
	tail, err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream, Values: map[string]any{"payload": "{}"},
	}).Result()
	if err != nil {
		t.Fatalf("add tail entry: %v", err)
	}

	result, err := PurgeStream(ctx, client, stream)
	if err != nil {
		t.Fatalf("PurgeStream: %v", err)
	}
	if result.Dropped < 5 {
		t.Errorf("dropped = %d, want at least the 5 seeded entries", result.Dropped)
	}
	if len(result.GroupsFastForwarded) != 1 || result.GroupsFastForwarded[0] != group {
		t.Errorf("GroupsFastForwarded = %v, want [%s]", result.GroupsFastForwarded, group)
	}

	if n, err := client.XLen(ctx, stream).Result(); err != nil || n != 0 {
		t.Errorf("stream length = %d (err %v), want 0", n, err)
	}

	groups, err := client.XInfoGroups(ctx, stream).Result()
	if err != nil {
		t.Fatalf("XInfoGroups after purge: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("group count = %d, want 1 — the group must survive the purge", len(groups))
	}
	g := groups[0]
	if g.Name != group {
		t.Errorf("group name = %q, want %q", g.Name, group)
	}
	if g.Pending != 0 {
		t.Errorf("group pending = %d, want 0 — the purge must clear the PEL", g.Pending)
	}
	if g.LastDeliveredID != tail {
		t.Errorf("group last-delivered-id = %q, want the stream tail %q ($)", g.LastDeliveredID, tail)
	}

	// A consumer reading new work resumes cleanly: nothing redelivered.
	seedPastEntry(t, client, stream, 0)
	msgs, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: group, Consumer: "metarr-server", Streams: []string{stream, ">"}, Count: 10,
	}).Result()
	if err != nil {
		t.Fatalf("post-purge read: %v", err)
	}
	if len(msgs) != 1 || len(msgs[0].Messages) != 1 {
		t.Fatalf("post-purge read delivered %v, want exactly the one new entry", msgs)
	}
}

// "all" purges every discovered durable stream, including one whose group has
// a backlog and one with no consumer group at all.
func TestPurgeAllStreamsFastForwardsEveryGroupOnRealRedis(t *testing.T) {
	client := realRedisClient(t)
	ctx := context.Background()

	grouped := SystemConfigUpdateStream
	groupedName := SystemConfigUpdateGroup
	ungrouped := AgentScanResultStream
	agentStream := AgentCommandStream("nas-01")
	agentGroup := AgentCommandGroup("nas-01")

	for i := 0; i < 3; i++ {
		seedPastEntry(t, client, grouped, time.Duration(200-i*10)*time.Second)
	}
	if err := client.XGroupCreate(ctx, grouped, groupedName, "0").Err(); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: groupedName, Consumer: "metarr-server", Streams: []string{grouped, ">"},
	}).Err(); err != nil {
		t.Fatalf("read into group: %v", err)
	}

	seedPastEntry(t, client, ungrouped, 30*time.Second) // no group ever created

	seedPastEntry(t, client, agentStream, 20*time.Second)
	if err := client.XGroupCreate(ctx, agentStream, agentGroup, "0").Err(); err != nil {
		t.Fatalf("create agent group: %v", err)
	}

	results, err := PurgeAllStreams(ctx, client)
	if err != nil {
		t.Fatalf("PurgeAllStreams: %v", err)
	}

	byStream := map[string]StreamPurge{}
	for _, r := range results {
		byStream[r.Stream] = r
	}
	for _, want := range []string{grouped, ungrouped, AgentNodeResultStream, agentStream} {
		if _, ok := byStream[want]; !ok {
			t.Errorf("stream %q was not in the purge-all results", want)
		}
	}
	if got := byStream[grouped].Dropped; got != 3 {
		t.Errorf("%s dropped = %d, want 3", grouped, got)
	}
	if len(byStream[ungrouped].GroupsFastForwarded) != 0 {
		t.Errorf("%s has no group, want no fast-forward, got %v", ungrouped, byStream[ungrouped].GroupsFastForwarded)
	}

	for _, s := range []string{grouped, agentStream} {
		groups, err := client.XInfoGroups(ctx, s).Result()
		if err != nil {
			t.Fatalf("XInfoGroups(%s): %v", s, err)
		}
		if len(groups) != 1 || groups[0].Pending != 0 {
			t.Errorf("%s groups after purge-all = %+v, want one group with zero pending", s, groups)
		}
	}
}
