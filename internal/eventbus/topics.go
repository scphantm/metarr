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

	// DirectoryScanStream is the Redis Stream the directory_scan event is fired
	// on when a configured scan directory is asked to be scanned. The event
	// payload is the resolved appconfig.ScanDirectory.
	DirectoryScanStream = "events.directory_scan"
	// DirectoryScanGroup is the consumer group used to read
	// DirectoryScanStream.
	DirectoryScanGroup = "directory_scan_group"
	// DirectoryScanEventName is the event name carried in the Event envelope
	// for a directory_scan event.
	DirectoryScanEventName = "directory_scan"
)
