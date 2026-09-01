//go:build realredis

// These tests need a real Redis: miniredis does not maintain a stream's
// entries-added counter or a consumer group's entries-read counter, so the
// exact publish/consume-rate delta can only be exercised against the real
// server. Run them with:
//
//	go test -tags realredis ./internal/server/busstats/...
//
// pointing REDIS_ADDR / REDIS_PASSWORD at a throwaway instance (defaults
// 127.0.0.1:6379 and the docker-compose dev password "metarr", database 15,
// which the test flushes).
package busstats

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"

	"Metarr/internal/shared/eventbus"
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

// Publish rate is the delta of the stream's entries-added counter between two
// sampler passes, and consume rate the delta of the group's entries-read —
// neither is the absolute count, and the first pass that sees a stream or
// group reports zero.
func TestRatesAreDeltaAcrossPassesOnRealRedis(t *testing.T) {
	client := realRedisClient(t)
	ctx := context.Background()
	sampler := New(client, nil, WithHistory(16))

	stream := eventbus.AgentScanResultStream
	group := eventbus.AgentScanResultGroup

	add := func(n int) {
		for i := 0; i < n; i++ {
			if err := client.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: map[string]any{"payload": "{}"}}).Err(); err != nil {
				t.Fatalf("xadd: %v", err)
			}
		}
	}

	add(2)
	sampler.pass(ctx) // first sighting of the stream — no prior sample
	if got := findStream(sampler.Get().GetStreams(), stream).GetPublishRate(); got != 0 {
		t.Fatalf("first pass publish_rate = %d, want 0 (not the absolute count)", got)
	}

	add(5)
	sampler.pass(ctx)
	if got := findStream(sampler.Get().GetStreams(), stream).GetPublishRate(); got != 5 {
		t.Fatalf("publish_rate = %d, want 5 (entries added since the previous pass)", got)
	}

	if err := client.XGroupCreate(ctx, stream, group, "0").Err(); err != nil {
		t.Fatalf("create group: %v", err)
	}
	readN := func(n int64) {
		if err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: group, Consumer: "metarr-server", Streams: []string{stream, ">"}, Count: n,
		}).Err(); err != nil {
			t.Fatalf("xreadgroup: %v", err)
		}
	}

	readN(3)
	sampler.pass(ctx) // first sighting of the group — consume_rate zero
	groupRate := func() int64 {
		st := findStream(sampler.Get().GetStreams(), stream)
		if len(st.GetGroups()) != 1 {
			t.Fatalf("want one group row, got %d", len(st.GetGroups()))
		}
		return st.GetGroups()[0].GetConsumeRate()
	}
	if got := groupRate(); got != 0 {
		t.Fatalf("first pass consume_rate = %d, want 0", got)
	}

	readN(2)
	sampler.pass(ctx)
	if got := groupRate(); got != 2 {
		t.Fatalf("consume_rate = %d, want 2 (entries read since the previous pass)", got)
	}

	// No new work between passes: both rates fall back to zero, not the
	// running totals.
	sampler.pass(ctx)
	st := findStream(sampler.Get().GetStreams(), stream)
	if st.GetPublishRate() != 0 || groupRate() != 0 {
		t.Fatalf("idle pass rates = (%d, %d), want (0, 0)", st.GetPublishRate(), groupRate())
	}
}
