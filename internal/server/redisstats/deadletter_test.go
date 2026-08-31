package redisstats

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"Metarr/internal/shared/eventbus"
)

func newTestCollector(t *testing.T) (*Collector, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return New(client), server
}

func TestCollectDeadLetterReportsNothingBeforeAMessageIsParked(t *testing.T) {
	collector, _ := newTestCollector(t)

	stat := collector.collectDeadLetter(context.Background())

	if stat.GetExists() || stat.GetLength() != 0 {
		t.Errorf("expected an empty dead-letter stat, got %+v", stat)
	}
	if stat.GetError() != "" {
		t.Errorf("empty dead-letter stream is not an error, got %q", stat.GetError())
	}
	if stat.GetNewestEntry() != nil {
		t.Errorf("expected no newest_entry on an empty stream, got %v", stat.GetNewestEntry())
	}
}

func TestCollectDeadLetterReportsLengthAndNewestEntry(t *testing.T) {
	collector, server := newTestCollector(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	before := time.Now().Add(-time.Second)
	for range 3 {
		if err := client.XAdd(context.Background(), &redis.XAddArgs{
			Stream: eventbus.DeadLetterStream,
			Values: map[string]any{"payload": `{"name":"x"}`},
		}).Err(); err != nil {
			t.Fatalf("XAdd: %v", err)
		}
	}
	after := time.Now().Add(time.Second)

	stat := collector.collectDeadLetter(context.Background())

	if !stat.GetExists() {
		t.Error("expected exists=true once messages are parked")
	}
	if stat.GetLength() != 3 {
		t.Errorf("length = %d, want 3", stat.GetLength())
	}
	newest := stat.GetNewestEntry().AsTime()
	if newest.Before(before) || newest.After(after) {
		t.Errorf("newest_entry %v is not within [%v, %v]", newest, before, after)
	}
}

func TestTimeFromStreamID(t *testing.T) {
	when, ok := timeFromStreamID("1700000000000-0")
	if !ok {
		t.Fatal("expected a well-formed stream ID to parse")
	}
	if got := when.UnixMilli(); got != 1700000000000 {
		t.Errorf("UnixMilli = %d, want 1700000000000", got)
	}
	if _, ok := timeFromStreamID("not-an-id"); ok {
		t.Error("expected a malformed stream ID to be rejected")
	}
}
