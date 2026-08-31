package eventbus

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// This file is where every event-bus name is defined once: Redis Stream
// names, consumer-group names, Pub/Sub channel names, the per-agent names
// addressed by slug, and the event-name discriminators carried in the
// envelope. It also holds the one stream topic table (StreamTopics) that is
// the single representation of every durable stream. agentproto and every
// listener, handler, and service import from here, so a name is declared
// once and the two sides of a stream can never drift out of step.

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

	// AgentNodeResultStream carries workflow node execution outcomes from the
	// agent back to the server. The name is reserved here so retention and
	// stats treat it as a durable stream from the start; its consumer group
	// and listener are created by the workflow-engine work (docs/adr/0006,
	// spec scphantm/metarr#37).
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

// StreamTopic is one durable Redis Stream in the system, and the single
// representation of it. Every inventory that used to keep its own list —
// the statistics dashboard, the retention sweep, the publish cap, per-agent
// discovery — reads StreamTopics() instead, so adding a durable stream is
// one row here rather than an edit in four places.
type StreamTopic struct {
	// Name is the literal stream name, or the glob for a pattern topic.
	Name string
	// Pattern is true when Name is a glob: the concrete topics come from
	// DiscoverStreamTopics expanding it against live Redis, not from this
	// row directly.
	Pattern bool
	// Group is the consumer group that reads the stream. It is "" when
	// nothing consumes the stream (a reserved name), and "" on a pattern
	// row (the group is per concrete stream, filled in by discovery).
	Group string
	// Consumed is true when a listener is registered on the stream. A
	// reserved-but-unconsumed stream — AgentNodeResultStream until the
	// workflow engine lands — is Consumed false with no Group.
	Consumed bool
	// Events are the envelope Name discriminators a handler on this stream
	// may see. Informational only: routing is the handler's job, not this
	// list's.
	Events []string
}

// streamScanCount is the COUNT hint for the per-agent stream SCAN. It only
// tunes how many keys Redis returns per round trip; the iterator still walks
// the whole keyspace.
const streamScanCount = 100

// StreamTopics is the one table of every durable stream: the static rows,
// plus one pattern row for the per-agent command streams.
// DiscoverStreamTopics expands the pattern against live Redis.
func StreamTopics() []StreamTopic {
	return []StreamTopic{
		SystemConfigUpdateTopic(),
		AgentScanResultTopic(),
		agentNodeResultTopic(),
		agentCommandStreamPatternTopic(),
	}
}

// streamTopicPublishable reports whether StreamBus.Publish may append to
// topic. It returns an error for a pattern topic — a glob names many streams,
// not one — and for a non-pattern topic whose Name is not one the stream
// topic table resolves to, whether a static row or a concrete per-agent
// command stream the pattern row covers. The topic constructors are the
// primary safety; this is the backstop for a hand-built StreamTopic.
func streamTopicPublishable(topic StreamTopic) error {
	if topic.Pattern {
		return fmt.Errorf("eventbus: stream topic %q is a pattern; a glob is not publishable", topic.Name)
	}
	if !streamTopicNameKnown(topic.Name) {
		return fmt.Errorf("eventbus: unknown stream topic %q", topic.Name)
	}
	return nil
}

// streamTopicNameKnown reports whether name is a stream the topic table
// resolves to: a static row's literal name, or a concrete stream a pattern
// row's glob covers.
func streamTopicNameKnown(name string) bool {
	for _, topic := range StreamTopics() {
		if topic.Pattern {
			if matchesStreamGlob(topic.Name, name) {
				return true
			}
			continue
		}
		if topic.Name == name {
			return true
		}
	}
	return false
}

// matchesStreamGlob reports whether name matches a single-'*' glob, the only
// shape the stream topic table's pattern rows use. It is the same
// prefix/suffix split SlugFromAgentCommandStream does, kept general to the
// pattern string.
func matchesStreamGlob(pattern, name string) bool {
	prefix, suffix, ok := strings.Cut(pattern, "*")
	if !ok {
		return pattern == name
	}
	return len(name) >= len(prefix)+len(suffix) &&
		strings.HasPrefix(name, prefix) &&
		strings.HasSuffix(name, suffix)
}

// SystemConfigUpdateTopic is the stream the server's config-update listener
// registers on.
func SystemConfigUpdateTopic() StreamTopic {
	return StreamTopic{
		Name:     SystemConfigUpdateStream,
		Group:    SystemConfigUpdateGroup,
		Consumed: true,
		Events:   []string{SystemConfigUpdateEventName},
	}
}

// AgentScanResultTopic is the shared stream the server's scan-result
// listener registers on. Events names the discriminator the stream is about
// — the per-item result — not every discriminator its handler branches on
// (it also sees scan_complete and scan_failed); the field is informational.
func AgentScanResultTopic() StreamTopic {
	return StreamTopic{
		Name:     AgentScanResultStream,
		Group:    AgentScanResultGroup,
		Consumed: true,
		Events:   []string{AgentScanResultEventName},
	}
}

// AgentCommandTopic is the concrete per-agent command topic for slug: the
// row the agent registers its scan-command listener with. Discovery
// produces the same shape for a stream it finds by glob.
func AgentCommandTopic(slug string) StreamTopic {
	return StreamTopic{
		Name:     AgentCommandStream(slug),
		Group:    AgentCommandGroup(slug),
		Consumed: true,
		Events:   []string{AgentScanCommandEventName},
	}
}

// agentNodeResultTopic is the reserved workflow node-result stream. It has
// no consumer group and no listener until the workflow engine lands
// (scphantm/metarr#37); retention and stats still treat it as a durable
// stream so it is visible before then.
func agentNodeResultTopic() StreamTopic {
	return StreamTopic{Name: AgentNodeResultStream}
}

// agentCommandStreamPatternTopic is the single pattern row. Each agent
// reads its work from a stream named after its slug, so the concrete rows
// are discovered rather than listed.
func agentCommandStreamPatternTopic() StreamTopic {
	return StreamTopic{
		Name:     AgentCommandStreamPattern,
		Pattern:  true,
		Consumed: true,
		Events:   []string{AgentScanCommandEventName},
	}
}

// DiscoverStreamTopics returns every concrete durable stream topic: the
// static rows as-is, plus one row per Redis key matching a pattern row,
// each with its consumer group derived from the slug. On a SCAN failure it
// returns what it has plus the error — every caller logs the error and
// proceeds with the partial set, so a failed scan still trims or shows what
// it can.
func DiscoverStreamTopics(ctx context.Context, client redis.UniversalClient) ([]StreamTopic, error) {
	var (
		topics  []StreamTopic
		scanErr error
	)
	seen := map[string]bool{}

	for _, topic := range StreamTopics() {
		if topic.Pattern {
			continue
		}
		topics = append(topics, topic)
		seen[topic.Name] = true
	}

	for _, topic := range StreamTopics() {
		if !topic.Pattern {
			continue
		}
		iterator := client.Scan(ctx, 0, topic.Name, streamScanCount).Iterator()
		for iterator.Next(ctx) {
			name := iterator.Val()
			if seen[name] {
				continue
			}
			seen[name] = true
			topics = append(topics, StreamTopic{
				Name:     name,
				Group:    groupForAgentCommandStream(name),
				Consumed: topic.Consumed,
				Events:   topic.Events,
			})
		}
		if err := iterator.Err(); err != nil && scanErr == nil {
			scanErr = err
		}
	}

	return topics, scanErr
}

// groupForAgentCommandStream turns an agent command stream name into the
// consumer group that agent reads it with, composing the slug helpers. It
// returns "" when name is not an agent command stream. This is the one
// place the stream-to-group derivation lives; other packages call discovery
// rather than re-deriving it.
func groupForAgentCommandStream(name string) string {
	slug := SlugFromAgentCommandStream(name)
	if slug == "" {
		return ""
	}
	return AgentCommandGroup(slug)
}

// KnownPubSubChannels returns the fixed Pub/Sub channels. The per-request
// reply channels are deliberately absent: they are named for a correlation
// ID and exist only for the duration of one request.
func KnownPubSubChannels() []string {
	return []string{HeartbeatRequestChannel, LogChannel}
}
