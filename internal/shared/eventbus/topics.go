package eventbus

import "strings"

// This file is the single registry of every event-bus name: Redis Stream
// names, consumer-group names, Pub/Sub channel names, the per-agent names
// addressed by slug, and the event-name discriminators carried in the
// envelope. agentproto and every listener, handler, and service import from
// here, so a name is declared once and the two sides of a stream can never
// drift out of step.

// ConsumerName is the Redis Streams consumer identity every server-side
// stream consumer reads with. It is one constant rather than a per-listener
// string because Metarr runs exactly one metarr-server instance: the
// single-writer lock (docs/adr/0002) and the rebuilt event bus
// (docs/adr/0006) both assume a single server-side consumer, and a second
// identity here would split a consumer group's pending entries between two
// readers that each believe they own the whole backlog. Agent-side
// consumers use their slug instead.
const ConsumerName = "metarr-server"

// Fixed Pub/Sub channels.
const (
	// HeartbeatRequestChannel is the Pub/Sub channel the heartbeat handler
	// publishes to, and the heartbeat listener subscribes to.
	HeartbeatRequestChannel = "heartbeat.request"

	// LogChannel is the Pub/Sub channel every process publishes its structured
	// log records to. Pub/Sub rather than a Stream is deliberate: logs are
	// high-volume and loss-tolerant (missing a few lines during a shipper
	// restart is fine), which is exactly the trade-off Pub/Sub makes and a
	// durable Stream would not. Neither binary talks to OpenObserve directly:
	// only metarr-server subscribes here and forwards the records over HTTP to
	// Fluent Bit's http input (Fluent Bit has no Redis input). The server also
	// consumes this channel to feed the live-tail pane in the UI.
	LogChannel = "logs.app"
)

// Fixed Redis Streams and the consumer groups that read them.
const (
	// SystemConfigUpdateStream is the Redis Stream the system_config_update
	// event is fired on when the application config is changed via the API.
	SystemConfigUpdateStream = "events.system_config_update"
	// SystemConfigUpdateGroup is the consumer group used to read
	// SystemConfigUpdateStream.
	SystemConfigUpdateGroup = "system_config_update_group"

	// AgentScanResultStream is the shared Redis Stream the agents report their
	// scan results on. Scanning itself is addressed to one agent over its own
	// command stream (AgentCommandStream) because a filesystem only exists on
	// the machine that has it mounted; results come back on this shared one,
	// every message naming the agent that produced it.
	AgentScanResultStream = "events.agent_scan_results"
	// AgentScanResultGroup is the consumer group the server reads
	// AgentScanResultStream with.
	AgentScanResultGroup = "agent_scan_results_group"

	// DeadLetterStream is where the Router parks a message that errored past
	// the retry cap, with the failure reason recorded in its metadata. Nothing
	// consumes it: it is size-capped (docs/adr/0006) and read by hand with
	// redis-cli to diagnose a stuck handler, and a parked entry is replayed by
	// re-adding it to its origin stream once the cause is fixed. It has no
	// consumer group.
	DeadLetterStream = "events.dead_letter"

	// AgentNodeResultStream carries workflow node execution outcomes from the
	// agent back to the server. The name is reserved here so retention and
	// stats treat it as a high-volume result stream from the start; its
	// consumer group and listener are created by the workflow-engine work
	// (docs/adr/0006, spec scphantm/metarr#37).
	AgentNodeResultStream = "events.agent_node_results"
)

// Event-name discriminators carried in the Event envelope's Name field.
const (
	// SystemConfigUpdateEventName is carried by a system_config_update event.
	SystemConfigUpdateEventName = "system_config_update"

	// AgentScanCommandEventName asks an agent to walk one mapped directory.
	AgentScanCommandEventName = "agent.scan"
	// AgentScanResultEventName carries one scanned item directory back.
	AgentScanResultEventName = "agent.scan_result"
	// AgentScanCompleteEventName ends a scan run that reached the end.
	AgentScanCompleteEventName = "agent.scan_complete"
	// AgentScanFailedEventName reports a scan that could not run.
	AgentScanFailedEventName = "agent.scan_failed"
	// AgentNFOReadEventName asks an agent to read one NFO file from disk now.
	AgentNFOReadEventName = "agent.nfo_read"
)

// AgentCommandStream is the durable stream one agent reads its work from. It
// is per-agent because filesystems are machine-local: a scan of /mnt/tank
// only means anything on the machine that has it mounted, so the work has to
// be addressed rather than offered to whichever agent is free.
func AgentCommandStream(slug string) string { return "events.agent." + slug + ".commands" }

// AgentCommandGroup is the consumer group for an agent's command stream. One
// agent process is expected per slug, enforced by agentproto.LockKey.
func AgentCommandGroup(slug string) string { return "agent_" + slug + "_group" }

// AgentCommandStreamPattern matches every agent command stream, so the Redis
// statistics dashboard can discover them without knowing the slugs.
const AgentCommandStreamPattern = "events.agent.*.commands"

// SlugFromAgentCommandStream recovers the slug from a stream name produced by
// AgentCommandStream, or "" if stream is not an agent command stream. The
// stats collector uses it to label a stream it discovered by glob with the
// consumer group that reads it, without re-deriving the name shape by hand.
func SlugFromAgentCommandStream(stream string) string {
	prefix, suffix, ok := strings.Cut(AgentCommandStreamPattern, "*")
	if !ok || len(stream) <= len(prefix)+len(suffix) ||
		!strings.HasPrefix(stream, prefix) || !strings.HasSuffix(stream, suffix) {
		return ""
	}
	slug := stream[len(prefix) : len(stream)-len(suffix)]
	if strings.Contains(slug, ".") {
		return ""
	}
	return slug
}

// AgentConfigChangedChannel tells one agent its configuration has been
// rewritten and it should re-read its config key. Best effort: the agent
// also re-reads on a timer, so a notification lost while it was reconnecting
// costs a delay rather than a stale configuration forever.
func AgentConfigChangedChannel(slug string) string { return "agent.config.changed." + slug }

// AgentRequestChannel is the agent's Pub/Sub request channel, for calls
// where an HTTP caller is waiting on the answer and a durable stream would
// be the wrong shape. Replies go to the correlation-scoped ReplyChannel.
func AgentRequestChannel(slug string) string { return "agent." + slug + ".request" }

// StreamTopic describes one Redis Stream and the consumer group that reads
// it, pairing the constants above so callers that need to walk every stream
// — the statistics collector, for one — don't have to restate the names.
type StreamTopic struct {
	Stream    string
	Group     string
	EventName string
}

// FixedStreamNames is every Redis Stream with a name known ahead of time:
// the streams with a server consumer group (KnownStreams), plus the two that
// exist but nothing consumes — the reserved node-result stream and the
// dead-letter stream. Retention trims this set; the per-agent command
// streams, whose names depend on which agents exist, are discovered by glob
// on top of it. One list so adding a stream is one edit.
func FixedStreamNames() []string {
	names := make([]string, 0, len(KnownStreams())+2)
	for _, topic := range KnownStreams() {
		names = append(names, topic.Stream)
	}
	return append(names, AgentNodeResultStream, DeadLetterStream)
}

// KnownStreams returns every fixed Redis Stream the application publishes to.
func KnownStreams() []StreamTopic {
	return []StreamTopic{
		{
			Stream:    SystemConfigUpdateStream,
			Group:     SystemConfigUpdateGroup,
			EventName: SystemConfigUpdateEventName,
		},
		{
			Stream:    AgentScanResultStream,
			Group:     AgentScanResultGroup,
			EventName: AgentScanResultEventName,
		},
	}
}

// KnownStreamPatterns returns glob patterns matching streams whose names are
// not known ahead of time. Each agent reads its work from a stream named after
// its slug, so the statistics collector discovers them rather than listing them.
func KnownStreamPatterns() []string {
	return []string{AgentCommandStreamPattern}
}

// KnownPubSubChannels returns the fixed Pub/Sub channels. The per-request
// reply channels are deliberately absent: they are named for a correlation
// ID and exist only for the duration of one request.
func KnownPubSubChannels() []string {
	return []string{HeartbeatRequestChannel, LogChannel}
}
