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
	byName := map[string]Topic{}
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

// Every row StreamTopics() returns is a KindStream row, and a stream row
// never carries a ReplyName (that field is KindRequestReply only).
func TestStreamTopicsAreAllKindStream(t *testing.T) {
	for _, topic := range StreamTopics() {
		if topic.Kind != KindStream {
			t.Errorf("%s: Kind = %q, want %q", topic.Name, topic.Kind, KindStream)
		}
		if topic.ReplyName != "" {
			t.Errorf("%s: stream row has ReplyName %q", topic.Name, topic.ReplyName)
		}
	}
}

// AgentScanResultTopic lists all three discriminators an agent sends on the
// shared results stream, in send order.
func TestAgentScanResultTopicEventsAreAllThree(t *testing.T) {
	got := AgentScanResultTopic().Events
	want := []string{AgentScanResultEventName, AgentScanCompleteEventName, AgentScanFailedEventName}
	if len(got) != len(want) {
		t.Fatalf("Events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Events = %v, want %v", got, want)
		}
	}
}

// Topics() is the unified table: every StreamTopics() row unchanged, plus
// the two fixed Pub/Sub channels tagged with their non-stream kind.
func TestUnifiedTopicsTable(t *testing.T) {
	byName := map[string]Topic{}
	for _, topic := range Topics() {
		byName[topic.Name] = topic
	}

	for _, streamTopic := range StreamTopics() {
		got, ok := byName[streamTopic.Name]
		if !ok {
			t.Errorf("Topics() is missing stream row %s", streamTopic.Name)
			continue
		}
		if got.Kind != KindStream {
			t.Errorf("%s: Kind = %q, want %q", got.Name, got.Kind, KindStream)
		}
	}

	heartbeat, ok := byName[HeartbeatRequestChannel]
	if !ok {
		t.Fatalf("Topics() is missing the heartbeat channel")
	}
	if heartbeat.Kind != KindRequestReply || heartbeat.ReplyName != HeartbeatReplyEventName {
		t.Errorf("heartbeat row = %+v, want KindRequestReply with ReplyName %q", heartbeat, HeartbeatReplyEventName)
	}
	if heartbeat.Group != "" {
		t.Errorf("heartbeat row has Group %q; non-stream kinds carry no group", heartbeat.Group)
	}

	log, ok := byName[LogChannel]
	if !ok {
		t.Fatalf("Topics() is missing the log channel")
	}
	if log.Kind != KindNotify || log.Events != nil || log.ReplyName != "" {
		t.Errorf("log row = %+v, want KindNotify with nil Events and no ReplyName", log)
	}
}

// The per-agent Pub/Sub channel constructors carry the right kind, name,
// reply name, and (for request/reply) request discriminator.
func TestPerAgentChannelTopics(t *testing.T) {
	changed := AgentConfigChangedTopic("nas-01")
	if changed.Name != AgentConfigChangedChannel("nas-01") || changed.Kind != KindNotify {
		t.Errorf("AgentConfigChangedTopic = %+v", changed)
	}
	if changed.Events != nil || changed.ReplyName != "" || changed.Group != "" {
		t.Errorf("AgentConfigChangedTopic carries stream/reply fields: %+v", changed)
	}

	request := AgentRequestTopic("nas-01")
	if request.Name != AgentRequestChannel("nas-01") || request.Kind != KindRequestReply {
		t.Errorf("AgentRequestTopic = %+v", request)
	}
	if request.ReplyName != AgentNFOReadReplyEventName ||
		len(request.Events) != 1 || request.Events[0] != AgentNFOReadEventName {
		t.Errorf("AgentRequestTopic = %+v, want Events [%s] ReplyName %s", request, AgentNFOReadEventName, AgentNFOReadReplyEventName)
	}
}

// KnownPubSubChannels is now derived from the unified table; pin its result
// so the derivation cannot silently drop or reorder a channel.
func TestKnownPubSubChannelsUnchanged(t *testing.T) {
	got := KnownPubSubChannels()
	want := []string{HeartbeatRequestChannel, LogChannel}
	if len(got) != len(want) {
		t.Fatalf("KnownPubSubChannels() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("KnownPubSubChannels() = %v, want %v", got, want)
		}
	}
}

// streamTopicPublishable is the guard Bus.Publish applies: static rows and
// pattern-covered per-agent command streams pass; a pattern topic and an
// unknown name are rejected.
func TestStreamTopicPublishableGuard(t *testing.T) {
	ok := []Topic{
		SystemConfigUpdateTopic().Topic,
		AgentScanResultTopic().Topic,
		agentNodeResultTopic().Topic,
		AgentCommandTopic("nas-01").Topic,
		{Name: AgentScanResultStream}, // a hand-built row with just the name
	}
	for _, topic := range ok {
		if err := streamTopicPublishable(topic); err != nil {
			t.Errorf("streamTopicPublishable(%+v) = %v, want nil", topic, err)
		}
	}

	bad := []Topic{
		{Name: "events.not_a_real_stream"},
		{Name: AgentCommandStream("nas-01") + ".extra"},
		agentCommandStreamPatternTopic().Topic,
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

	var found *Topic
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

// AgentPubSubChannels is the non-stream slice of AgentTopics: exactly the
// config-changed notification and the request channel, in that order.
func TestAgentPubSubChannels(t *testing.T) {
	got := AgentPubSubChannels("nas-01")
	want := []string{"agent.config.changed.nas-01", "agent.nas-01.request"}
	if len(got) != len(want) {
		t.Fatalf("AgentPubSubChannels = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AgentPubSubChannels = %v, want %v", got, want)
		}
	}
}

// AgentTopics is the per-agent counterpart to Topics() the expected-topology
// derivation expands for each registered agent: the command stream, the
// config-changed notify channel, and the request/reply channel, each tagged
// by Kind so a caller can split them the same way it splits Topics().
func TestAgentTopics(t *testing.T) {
	got := AgentTopics("nas-01")
	want := []Topic{
		AgentCommandTopic("nas-01").Topic,
		AgentConfigChangedTopic("nas-01").Topic,
		AgentRequestTopic("nas-01").Topic,
	}
	if len(got) != len(want) {
		t.Fatalf("AgentTopics returned %d topics, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Name != want[i].Name || got[i].Kind != want[i].Kind {
			t.Errorf("AgentTopics[%d] = {%s, %s}, want {%s, %s}",
				i, got[i].Name, got[i].Kind, want[i].Name, want[i].Kind)
		}
	}

	// Exactly one KindStream row (the command stream); the rest are channels,
	// and the non-stream names match AgentPubSubChannels.
	var streams, channels []string
	for _, topic := range got {
		if topic.Kind == KindStream {
			streams = append(streams, topic.Name)
			continue
		}
		channels = append(channels, topic.Name)
	}
	if len(streams) != 1 || streams[0] != AgentCommandStream("nas-01") {
		t.Errorf("AgentTopics streams = %v, want [%s]", streams, AgentCommandStream("nas-01"))
	}
	psc := AgentPubSubChannels("nas-01")
	if len(channels) != len(psc) {
		t.Fatalf("AgentTopics channels = %v, want %v", channels, psc)
	}
	for i := range psc {
		if channels[i] != psc[i] {
			t.Errorf("AgentTopics channels = %v, want %v", channels, psc)
		}
	}
}

// SlugFromAgentSource is the inverse of AgentSource and rejects the server
// identity and a bare prefix.
func TestSlugFromAgentSource(t *testing.T) {
	if slug, ok := SlugFromAgentSource(AgentSource("nas-01")); !ok || slug != "nas-01" {
		t.Errorf("SlugFromAgentSource(%q) = %q, %v; want nas-01, true", AgentSource("nas-01"), slug, ok)
	}
	for _, bad := range []string{SourceServer, "metarr-agent-", "nas-01", ""} {
		if slug, ok := SlugFromAgentSource(bad); ok || slug != "" {
			t.Errorf("SlugFromAgentSource(%q) = %q, %v; want \"\", false", bad, slug, ok)
		}
	}
}

func wantStreamTopic(StreamTopic)   {}
func wantNotifyTopic(NotifyTopic)   {}
func wantRequestTopic(RequestTopic) {}

// The topic constructors return the wrapper that matches their Kind, so a
// verb handed the wrong one is a compile error rather than a runtime
// ErrWrongKind. These calls fail to build if a constructor's return type is
// ever widened back to a bare Topic or changed to the wrong wrapper.
func TestTopicConstructorsReturnTheKindTypedWrapper(t *testing.T) {
	wantStreamTopic(SystemConfigUpdateTopic())
	wantStreamTopic(AgentScanResultTopic())
	wantStreamTopic(AgentCommandTopic("nas-01"))
	wantNotifyTopic(LogTopic())
	wantNotifyTopic(AgentConfigChangedTopic("nas-01"))
	wantRequestTopic(HeartbeatTopic())
	wantRequestTopic(AgentRequestTopic("nas-01"))

	// The embedded row still carries the matching Kind.
	if SystemConfigUpdateTopic().Kind != KindStream ||
		LogTopic().Kind != KindNotify ||
		HeartbeatTopic().Kind != KindRequestReply {
		t.Fatal("a wrapper's embedded Topic.Kind disagrees with its wrapper type")
	}
}

func containsTopic(topics []Topic, name string) bool {
	for _, topic := range topics {
		if topic.Name == name {
			return true
		}
	}
	return false
}
