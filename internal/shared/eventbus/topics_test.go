package eventbus

import (
	"strings"
	"testing"
)

// The per-agent stream name and the discovery pattern the stats collector
// scans with have to agree, or the dashboard silently shows no agent streams.
func TestAgentCommandStreamMatchesTheDiscoveryPattern(t *testing.T) {
	if got, want := AgentCommandStream("nas-01"), "events.agent.nas-01.commands"; got != want {
		t.Errorf("AgentCommandStream = %q, want %q", got, want)
	}

	prefix := strings.TrimSuffix(AgentCommandStreamPattern, "*.commands")
	suffix := ".commands"
	stream := AgentCommandStream("nas-01")
	if !strings.HasPrefix(stream, prefix) || !strings.HasSuffix(stream, suffix) {
		t.Errorf("AgentCommandStream(%q) = %q does not match pattern %q", "nas-01", stream, AgentCommandStreamPattern)
	}

	if got, want := KnownStreamPatterns(), []string{AgentCommandStreamPattern}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("KnownStreamPatterns = %v, want %v", got, want)
	}
}

func TestSlugFromAgentCommandStreamRoundTrips(t *testing.T) {
	for _, slug := range []string{"nas-01", "a", "local_agent", "agent-with-a-long-name-99"} {
		if got := SlugFromAgentCommandStream(AgentCommandStream(slug)); got != slug {
			t.Errorf("SlugFromAgentCommandStream(AgentCommandStream(%q)) = %q", slug, got)
		}
	}
}

func TestSlugFromAgentCommandStreamRejectsNonMatches(t *testing.T) {
	for _, stream := range []string{
		"",
		"events.agent..commands",
		"events.agent.nas.01.commands",
		"events.agent_scan_results",
		"events.system_config_update",
		"events.agent.nas-01.results",
	} {
		if got := SlugFromAgentCommandStream(stream); got != "" {
			t.Errorf("SlugFromAgentCommandStream(%q) = %q, want empty", stream, got)
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

// KnownStreams must name its event with the shared constant, not a literal,
// so the stats collector's EventName label cannot drift from what publishers
// actually put on the wire.
func TestKnownStreamsUseSharedEventNameConstants(t *testing.T) {
	for _, topic := range KnownStreams() {
		switch topic.Stream {
		case SystemConfigUpdateStream:
			if topic.EventName != SystemConfigUpdateEventName {
				t.Errorf("system_config_update EventName = %q", topic.EventName)
			}
		case AgentScanResultStream:
			if topic.EventName != AgentScanResultEventName {
				t.Errorf("agent_scan_results EventName = %q", topic.EventName)
			}
		default:
			t.Errorf("unexpected known stream %q", topic.Stream)
		}
	}
}
