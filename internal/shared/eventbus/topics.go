package eventbus

// Topic and event-name constants shared between the HTTP handlers that
// publish events and the listeners that consume them.
const (
	// HeartbeatRequestChannel is the Pub/Sub channel the heartbeat handler
	// publishes to, and the heartbeat listener subscribes to.
	HeartbeatRequestChannel = "heartbeat.request"

	// SonarrCacheDataStream is the Redis Stream the sonarr_cache_data task
	// event is fired on.
	SonarrCacheDataStream = "events.sonarr_cache_data"
	// SonarrCacheDataGroup is the consumer group used to read
	// SonarrCacheDataStream.
	SonarrCacheDataGroup = "sonarr_cache_data_group"
	// SonarrCacheDataEventName is the event name carried in the Event
	// envelope for a sonarr_cache_data task.
	SonarrCacheDataEventName = "sonarr_cache_data"

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
			Stream:    SonarrCacheDataStream,
			Group:     SonarrCacheDataGroup,
			EventName: SonarrCacheDataEventName,
		},
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
	return []string{HeartbeatRequestChannel}
}
