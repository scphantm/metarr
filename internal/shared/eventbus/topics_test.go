package eventbus

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
)

// One test over the stream topic table, replacing the four per-helper tests
// this file used to carry. It exercises the table through its public surface:
// the static rows, the reserved row, and discovery against miniredis.
func TestStreamTopicTable(t *testing.T) {
	byName := map[string]StreamTopic{}
	for _, topic := range StreamTopics() {
		byName[topic.Name] = topic
	}

	// A row carries a consumer group exactly when a listener consumes it and
	// it is not a pattern row (a pattern's group is per concrete stream).
	for _, topic := range StreamTopics() {
		wantGroup := topic.Consumed && !topic.Pattern
		if got := topic.Group != ""; got != wantGroup {
			t.Errorf("%s: Group=%q with Consumed=%v Pattern=%v", topic.Name, topic.Group, topic.Consumed, topic.Pattern)
		}
	}

	// The reserved node-result stream is the one unconsumed row: visible to
	// retention and stats, but no group and no listener until #37.
	node, ok := byName[AgentNodeResultStream]
	if !ok {
		t.Fatalf("%s is not in the stream topic table", AgentNodeResultStream)
	}
	if node.Consumed || node.Group != "" {
		t.Errorf("%s should be reserved-unconsumed, got %+v", AgentNodeResultStream, node)
	}

	// Exactly one pattern row, for the per-agent command streams.
	patterns := 0
	for _, topic := range StreamTopics() {
		if topic.Pattern {
			patterns++
			if topic.Name != AgentCommandStreamPattern {
				t.Errorf("pattern row Name = %q, want %q", topic.Name, AgentCommandStreamPattern)
			}
		}
	}
	if patterns != 1 {
		t.Errorf("stream topic table has %d pattern rows, want 1", patterns)
	}
}

// streamTopicPublishable is the guard StreamBus.Publish applies: static
// rows and pattern-covered per-agent command streams pass; a pattern topic
// and an unknown name are rejected.
func TestStreamTopicPublishableGuard(t *testing.T) {
	ok := []StreamTopic{
		SystemConfigUpdateTopic(),
		AgentScanResultTopic(),
		agentNodeResultTopic(),
		AgentCommandTopic("nas-01"),
		{Name: AgentScanResultStream}, // a hand-built row with just the name
	}
	for _, topic := range ok {
		if err := streamTopicPublishable(topic); err != nil {
			t.Errorf("streamTopicPublishable(%+v) = %v, want nil", topic, err)
		}
	}

	bad := []StreamTopic{
		{Name: "events.not_a_real_stream"},
		{Name: AgentCommandStream("nas-01") + ".extra"},
		agentCommandStreamPatternTopic(),
		{Name: AgentScanResultStream, Pattern: true}, // Pattern flag alone is disqualifying
	}
	for _, topic := range bad {
		if err := streamTopicPublishable(topic); err == nil {
			t.Errorf("streamTopicPublishable(%+v) = nil, want an error", topic)
		}
	}
}

func TestDiscoverStreamTopicsExpandsPerAgentStreams(t *testing.T) {
	_, client := newRetentionRedis(t)
	ctx := context.Background()

	// Seed a per-agent command stream so discovery has something to expand.
	if err := client.XAdd(ctx, &redis.XAddArgs{
		Stream: AgentCommandStream("nas-01"),
		Values: map[string]any{"payload": "{}"},
	}).Err(); err != nil {
		t.Fatalf("seed per-agent stream: %v", err)
	}

	topics, err := DiscoverStreamTopics(ctx, client)
	if err != nil {
		t.Fatalf("DiscoverStreamTopics: %v", err)
	}

	var found *StreamTopic
	for i := range topics {
		if topics[i].Pattern {
			t.Errorf("discovery returned a pattern row: %+v", topics[i])
		}
		if topics[i].Name == AgentCommandStream("nas-01") {
			found = &topics[i]
		}
	}

	if found == nil {
		t.Fatalf("discovery did not expand %s", AgentCommandStream("nas-01"))
	}
	if !found.Consumed {
		t.Errorf("expanded per-agent row should be Consumed")
	}
	if found.Group != AgentCommandGroup("nas-01") {
		t.Errorf("expanded per-agent row Group = %q, want %q", found.Group, AgentCommandGroup("nas-01"))
	}

	// The static rows are still present, unchanged.
	if !containsTopic(topics, SystemConfigUpdateStream) || !containsTopic(topics, AgentNodeResultStream) {
		t.Errorf("discovery dropped a static row: %+v", topics)
	}
}

// The per-agent stream name and the group it derives to round-trip through
// the slug, so a stream discovered by glob still reports the group that reads
// it.
func TestSlugAndGroupRoundTrip(t *testing.T) {
	for _, slug := range []string{"nas-01", "a", "local_agent", "agent-with-a-long-name-99"} {
		stream := AgentCommandStream(slug)
		if got := SlugFromAgentCommandStream(stream); got != slug {
			t.Errorf("SlugFromAgentCommandStream(%q) = %q, want %q", stream, got, slug)
		}
		if got := groupForAgentCommandStream(stream); got != AgentCommandGroup(slug) {
			t.Errorf("groupForAgentCommandStream(%q) = %q, want %q", stream, got, AgentCommandGroup(slug))
		}
	}

	for _, notAStream := range []string{
		"", "events.agent..commands", "events.agent.nas.01.commands",
		"events.agent_scan_results", "events.system_config_update",
	} {
		if got := groupForAgentCommandStream(notAStream); got != "" {
			t.Errorf("groupForAgentCommandStream(%q) = %q, want empty", notAStream, got)
		}
	}
}

// ConsumerName is the single server-side consumer identity. Pin the value so
// a rename is a deliberate, reviewed change rather than a silent one that
// orphans a consumer group's pending entries.
func TestConsumerNameIsTheSingleServerIdentity(t *testing.T) {
	if ConsumerName != "metarr-server" {
		t.Errorf("ConsumerName = %q, want %q", ConsumerName, "metarr-server")
	}
}

func TestAgentNameBuildersAreStable(t *testing.T) {
	cases := map[string]string{
		AgentCommandGroup("nas-01"):         "agent_nas-01_group",
		AgentConfigChangedChannel("nas-01"): "agent.config.changed.nas-01",
		AgentRequestChannel("nas-01"):       "agent.nas-01.request",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func containsTopic(topics []StreamTopic, name string) bool {
	for _, topic := range topics {
		if topic.Name == name {
			return true
		}
	}
	return false
}
