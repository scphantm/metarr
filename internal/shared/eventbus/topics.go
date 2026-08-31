package eventbus

// Topic and event-name constants shared between the HTTP handlers that
// publish events and the listeners that consume them.
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

	// SystemConfigUpdateStream is the Redis Stream the system_config_update
	// event is fired on when the application config is changed via the API.
	SystemConfigUpdateStream = "events.system_config_update"
	// SystemConfigUpdateGroup is the consumer group used to read
	// SystemConfigUpdateStream.
	SystemConfigUpdateGroup = "system_config_update_group"
	// SystemConfigUpdateEventName is the event name carried in the Event
	// envelope for a system_config_update event.
	SystemConfigUpdateEventName = "system_config_update"

	// AgentScanResultStream is the Redis Stream the agents report their scan
	// results on. Scanning itself is addressed to one agent over its own
	// command stream — see agentproto — because a filesystem only exists on the
	// machine that has it mounted; results come back on this shared one.
	AgentScanResultStream = "events.agent_scan_results"
	// AgentScanResultGroup is the consumer group the server reads
	// AgentScanResultStream with.
	AgentScanResultGroup = "agent_scan_results_group"
)

// StreamTopic describes one Redis Stream and the consumer group that reads
// it, pairing the constants above so callers that need to walk every stream
// — the statistics collector, for one — don't have to restate the names.
type StreamTopic struct {
	Stream    string
	Group     string
	EventName string
}

// KnownStreams returns every Redis Stream the application publishes to.
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
			EventName: "agent.scan_result",
		},
	}
}

// KnownStreamPatterns returns glob patterns matching streams whose names are
// not known ahead of time. Each agent reads its work from a stream named after
// its slug, so the statistics collector discovers them rather than listing them.
func KnownStreamPatterns() []string {
	return []string{"events.agent.*.commands"}
}

// KnownPubSubChannels returns the fixed Pub/Sub channels. The per-request
// reply channels are deliberately absent: they are named for a correlation
// ID and exist only for the duration of one request.
func KnownPubSubChannels() []string {
	return []string{HeartbeatRequestChannel, LogChannel}
}
