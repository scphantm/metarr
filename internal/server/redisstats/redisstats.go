// Package redisstats collects a point-in-time snapshot of the Redis instance
// backing the event system: the depth and consumer-group state of every event
// stream, the subscriber count of every Pub/Sub channel, and a handful of
// server-wide counters.
//
// One thing here is worth stating up front because it shapes what a caller
// can honestly display: the event streams and the Pub/Sub channels are not
// two flavours of the same thing. Streams are durable — messages sit on them
// until acknowledged, so depth and pending counts are real numbers. Pub/Sub
// is fire-and-forget: it holds nothing, and a message published with no
// subscriber attached is simply gone. So ChannelStat carries a subscriber
// count and no depth, because no depth exists to report.
package redisstats

import (
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"Metarr/internal/shared/eventbus"
)

// Collector reads statistics from a Redis instance.
type Collector struct {
	client redis.UniversalClient
}

// New wraps client as a Collector.
func New(client redis.UniversalClient) *Collector {
	return &Collector{client: client}
}

// Snapshot is everything the collector knows at one instant.
type Snapshot struct {
	CollectedAt time.Time     `json:"collected_at"`
	Server      ServerInfo    `json:"server"`
	Streams     []StreamStat  `json:"streams"`
	PubSub      []ChannelStat `json:"pubsub"`
}

// ServerInfo holds the instance-wide counters, read from INFO and DBSIZE.
type ServerInfo struct {
	Version          string `json:"version"`
	UptimeSeconds    int64  `json:"uptime_seconds"`
	ConnectedClients int64  `json:"connected_clients"`
	UsedMemory       int64  `json:"used_memory"`
	UsedMemoryHuman  string `json:"used_memory_human"`
	OpsPerSecond     int64  `json:"ops_per_second"`
	TotalKeys        int64  `json:"total_keys"`
}

// StreamStat is one event stream and the consumer groups reading it.
type StreamStat struct {
	Stream    string `json:"stream"`
	EventName string `json:"event_name"`
	Length    int64  `json:"length"`
	// Exists distinguishes an empty stream from one that has never been
	// created. Streams are created lazily, when a listener first subscribes,
	// so a length of zero on its own is ambiguous.
	Exists bool        `json:"exists"`
	Groups []GroupStat `json:"groups"`
	// Error records a per-stream failure. One unreadable stream should not
	// cost the caller the rest of the snapshot.
	Error string `json:"error,omitempty"`
}

// GroupStat is one consumer group's position on a stream.
type GroupStat struct {
	Name            string         `json:"name"`
	Consumers       int64          `json:"consumers"`
	Pending         int64          `json:"pending"`
	Lag             int64          `json:"lag"`
	LastDeliveredID string         `json:"last_delivered_id"`
	ConsumerDetail  []ConsumerStat `json:"consumer_detail"`
}

// ConsumerStat is a single consumer within a group.
type ConsumerStat struct {
	Name        string `json:"name"`
	Pending     int64  `json:"pending"`
	IdleSeconds int64  `json:"idle_seconds"`
}

// ChannelStat is one Pub/Sub channel. It carries no message count because
// Redis Pub/Sub queues nothing — see the package comment.
type ChannelStat struct {
	Channel     string `json:"channel"`
	Subscribers int64  `json:"subscribers"`
	// Known marks the application's declared channels. Channels discovered
	// at runtime — the per-correlation-id reply channels — come back false,
	// since they exist only while a request is in flight.
	Known bool `json:"known"`
}

// Collect gathers a snapshot. Errors reading an individual stream are
// recorded on that stream rather than returned; only a failure to reach the
// server at all is returned as an error.
func (c *Collector) Collect(ctx context.Context) (Snapshot, error) {
	server, err := c.collectServer(ctx)
	if err != nil {
		return Snapshot{}, err
	}

	pubsub, err := c.collectPubSub(ctx)
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		CollectedAt: time.Now().UTC(),
		Server:      server,
		Streams:     c.collectStreams(ctx),
		PubSub:      pubsub,
	}, nil
}

func (c *Collector) collectServer(ctx context.Context) (ServerInfo, error) {
	raw, err := c.client.Info(ctx, "server", "clients", "memory", "stats").Result()
	if err != nil {
		return ServerInfo{}, err
	}
	fields := parseInfo(raw)

	keys, err := c.client.DBSize(ctx).Result()
	if err != nil {
		return ServerInfo{}, err
	}

	return ServerInfo{
		Version:          fields["redis_version"],
		UptimeSeconds:    infoInt(fields, "uptime_in_seconds"),
		ConnectedClients: infoInt(fields, "connected_clients"),
		UsedMemory:       infoInt(fields, "used_memory"),
		UsedMemoryHuman:  fields["used_memory_human"],
		OpsPerSecond:     infoInt(fields, "instantaneous_ops_per_sec"),
		TotalKeys:        keys,
	}, nil
}

func (c *Collector) collectStreams(ctx context.Context) []StreamStat {
	topics := eventbus.KnownStreams()
	topics = append(topics, c.discoverStreams(ctx, topics)...)

	stats := make([]StreamStat, 0, len(topics))
	for _, topic := range topics {
		stats = append(stats, c.collectStream(ctx, topic))
	}

	return stats
}

// discoverStreams finds streams whose names are not known ahead of time.
//
// Each agent reads its work from a stream named after its slug, so the set
// depends on which agents exist. Without this they would be invisible on the
// dashboard — which is exactly where someone would look to find out why an
// agent is not picking up work.
func (c *Collector) discoverStreams(ctx context.Context, known []eventbus.StreamTopic) []eventbus.StreamTopic {
	seen := make(map[string]bool, len(known))
	for _, topic := range known {
		seen[topic.Stream] = true
	}

	var discovered []eventbus.StreamTopic
	for _, pattern := range eventbus.KnownStreamPatterns() {
		iterator := c.client.Scan(ctx, 0, pattern, 100).Iterator()
		for iterator.Next(ctx) {
			stream := iterator.Val()
			if seen[stream] {
				continue
			}
			seen[stream] = true

			// The group name is derivable from the stream name, so a
			// discovered stream still reports its consumer group rather than
			// appearing as an orphan.
			discovered = append(discovered, eventbus.StreamTopic{
				Stream:    stream,
				Group:     groupForAgentStream(stream),
				EventName: "agent.scan",
			})
		}
		if err := iterator.Err(); err != nil {
			// Discovery is an enhancement to the dashboard, not the point of
			// it. A failed scan drops the dynamic streams rather than the
			// whole snapshot.
			return discovered
		}
	}
	return discovered
}

// groupForAgentStream turns "events.agent.<slug>.commands" into the consumer
// group that agent reads it with.
func groupForAgentStream(stream string) string {
	slug := strings.TrimSuffix(strings.TrimPrefix(stream, "events.agent."), ".commands")
	if slug == "" || slug == stream {
		return ""
	}
	return "agent_" + slug + "_group"
}

func (c *Collector) collectStream(ctx context.Context, topic eventbus.StreamTopic) StreamStat {
	stat := StreamStat{
		Stream:    topic.Stream,
		EventName: topic.EventName,
		Groups:    []GroupStat{},
	}

	length, err := c.client.XLen(ctx, topic.Stream).Result()
	if err != nil {
		stat.Error = err.Error()
		return stat
	}
	stat.Length = length

	groups, err := c.client.XInfoGroups(ctx, topic.Stream).Result()
	if err != nil {
		// A stream nobody has subscribed to yet does not exist at all, which
		// XINFO reports as a missing key. That is a normal cold-start state,
		// not a failure worth surfacing as one.
		if isMissingKey(err) {
			return stat
		}
		stat.Error = err.Error()
		return stat
	}
	stat.Exists = true

	for _, group := range groups {
		stat.Groups = append(stat.Groups, GroupStat{
			Name:            group.Name,
			Consumers:       group.Consumers,
			Pending:         group.Pending,
			Lag:             group.Lag,
			LastDeliveredID: group.LastDeliveredID,
			ConsumerDetail:  c.collectConsumers(ctx, topic.Stream, group.Name),
		})
	}

	return stat
}

func (c *Collector) collectConsumers(ctx context.Context, stream, group string) []ConsumerStat {
	consumers, err := c.client.XInfoConsumers(ctx, stream, group).Result()
	if err != nil {
		return []ConsumerStat{}
	}

	stats := make([]ConsumerStat, 0, len(consumers))
	for _, consumer := range consumers {
		stats = append(stats, ConsumerStat{
			Name:        consumer.Name,
			Pending:     consumer.Pending,
			IdleSeconds: int64(consumer.Idle.Seconds()),
		})
	}

	return stats
}

func (c *Collector) collectPubSub(ctx context.Context) ([]ChannelStat, error) {
	// PUBSUB CHANNELS only lists channels that currently have a subscriber,
	// so it finds the transient reply channels but would silently omit a
	// declared channel whose listener is down — exactly the case worth
	// seeing. Merging the declared list in fixes that, reporting zero
	// subscribers rather than dropping the row.
	active, err := c.client.PubSubChannels(ctx, "*").Result()
	if err != nil {
		return nil, err
	}

	known := eventbus.KnownPubSubChannels()
	names := append([]string{}, known...)
	for _, channel := range active {
		if !contains(names, channel) {
			names = append(names, channel)
		}
	}

	counts, err := c.client.PubSubNumSub(ctx, names...).Result()
	if err != nil {
		return nil, err
	}

	stats := make([]ChannelStat, 0, len(names))
	for _, name := range names {
		stats = append(stats, ChannelStat{
			Channel:     name,
			Subscribers: counts[name],
			Known:       contains(known, name),
		})
	}

	return stats, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// isMissingKey reports whether err is Redis's "no such key" error, which
// XINFO returns for a stream that has not been created yet.
func isMissingKey(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no such key")
}
