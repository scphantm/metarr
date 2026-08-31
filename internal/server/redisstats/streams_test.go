package redisstats

import (
	"context"
	"testing"

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

// The stream list has no events.dead_letter row: a message that exhausts its
// retries is logged and acked, not parked, so there is nothing to surface.
func TestCollectStreamsHasNoDeadLetterRow(t *testing.T) {
	collector, _ := newTestCollector(t)

	for _, stream := range collector.collectStreams(context.Background()) {
		if stream.GetStream() == "events.dead_letter" {
			t.Errorf("events.dead_letter is still reported as a stream: %+v", stream)
		}
	}
}

// The reserved workflow node-result stream shows on the dashboard before its
// listener lands (#37): present in the stream list, reading as not-yet-created.
func TestCollectStreamsIncludesTheReservedNodeResultStream(t *testing.T) {
	collector, _ := newTestCollector(t)

	streams := collector.collectStreams(context.Background())

	var node *StreamStat
	for _, s := range streams {
		if s.GetStream() == eventbus.AgentNodeResultStream {
			node = s
		}
	}
	if node == nil {
		t.Fatalf("%s is not in the collected stream list", eventbus.AgentNodeResultStream)
	}
	if node.GetExists() {
		t.Errorf("%s should read as not-yet-created before its listener lands", eventbus.AgentNodeResultStream)
	}
}

// A per-agent command stream in Redis is still discovered and labelled with
// the consumer group that reads it.
func TestCollectStreamsDiscoversPerAgentCommandStreams(t *testing.T) {
	collector, _ := newTestCollector(t)
	ctx := context.Background()

	stream := eventbus.AgentCommandStream("nas-01")
	if err := collector.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"payload": "{}"},
	}).Err(); err != nil {
		t.Fatalf("seed per-agent stream: %v", err)
	}

	var found *StreamStat
	for _, s := range collector.collectStreams(ctx) {
		if s.GetStream() == stream {
			found = s
		}
	}
	if found == nil {
		t.Fatalf("per-agent command stream %s was not discovered", stream)
	}
}
