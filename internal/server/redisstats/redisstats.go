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
	"google.golang.org/protobuf/types/known/timestamppb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
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

// The snapshot model. Every type here is an alias to the generated
// metarr.v1 message that defines it — proto is the single definition for a
// model that crosses a language boundary, and this one crosses the wire to
// the dashboard. See docs/adr/0005.
type (
	Snapshot     = metarrv1.RedisSnapshot
	ServerInfo   = metarrv1.RedisServerInfo
	StreamStat   = metarrv1.RedisStreamStat
	GroupStat    = metarrv1.RedisGroupStat
	ConsumerStat = metarrv1.RedisConsumerStat
	ChannelStat  = metarrv1.RedisChannelStat
)

// Collect gathers a snapshot. Errors reading an individual stream are
// recorded on that stream rather than returned; only a failure to reach the
// server at all is returned as an error.
func (c *Collector) Collect(ctx context.Context) (*Snapshot, error) {
	server, err := c.collectServer(ctx)
	if err != nil {
		return nil, err
	}

	pubsub, err := c.collectPubSub(ctx)
	if err != nil {
		return nil, err
	}

	return &Snapshot{
		CollectedAt: timestamppb.New(time.Now().UTC()),
		Server:      server,
		Streams:     c.collectStreams(ctx),
		Pubsub:      pubsub,
	}, nil
}

func (c *Collector) collectServer(ctx context.Context) (*ServerInfo, error) {
	raw, err := c.client.Info(ctx, "server", "clients", "memory", "stats").Result()
	if err != nil {
		return nil, err
	}
	fields := parseInfo(raw)

	keys, err := c.client.DBSize(ctx).Result()
	if err != nil {
		return nil, err
	}

	return &ServerInfo{
		Version:          fields["redis_version"],
		UptimeSeconds:    infoInt(fields, "uptime_in_seconds"),
		ConnectedClients: infoInt(fields, "connected_clients"),
		UsedMemory:       infoInt(fields, "used_memory"),
		UsedMemoryHuman:  fields["used_memory_human"],
		OpsPerSecond:     infoInt(fields, "instantaneous_ops_per_sec"),
		TotalKeys:        keys,
	}, nil
}

func (c *Collector) collectStreams(ctx context.Context) []*StreamStat {
	// One read over the shared stream topic table: the static rows plus one
	// concrete row per per-agent command stream currently in Redis. A
	// partial SCAN failure still yields the streams it did find — someone
	// looking here to see why an agent is not picking up work should see
	// what there is rather than a blank panel.
	topics, _ := eventbus.DiscoverStreamTopics(ctx, c.client)

	stats := make([]*StreamStat, 0, len(topics))
	for _, topic := range topics {
		stats = append(stats, c.collectStream(ctx, topic))
	}

	return stats
}

func (c *Collector) collectStream(ctx context.Context, topic eventbus.StreamTopic) *StreamStat {
	stat := &StreamStat{
		Stream:    topic.Name,
		EventName: strings.Join(topic.Events, ", "),
		Groups:    []*GroupStat{},
	}

	length, err := c.client.XLen(ctx, topic.Name).Result()
	if err != nil {
		stat.Error = err.Error()
		return stat
	}
	stat.Length = length

	groups, err := c.client.XInfoGroups(ctx, topic.Name).Result()
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
		stat.Groups = append(stat.Groups, &GroupStat{
			Name:            group.Name,
			Consumers:       group.Consumers,
			Pending:         group.Pending,
			Lag:             group.Lag,
			LastDeliveredId: group.LastDeliveredID,
			ConsumerDetail:  c.collectConsumers(ctx, topic.Name, group.Name),
		})
	}

	return stat
}

func (c *Collector) collectConsumers(ctx context.Context, stream, group string) []*ConsumerStat {
	consumers, err := c.client.XInfoConsumers(ctx, stream, group).Result()
	if err != nil {
		return []*ConsumerStat{}
	}

	stats := make([]*ConsumerStat, 0, len(consumers))
	for _, consumer := range consumers {
		stats = append(stats, &ConsumerStat{
			Name:        consumer.Name,
			Pending:     consumer.Pending,
			IdleSeconds: int64(consumer.Idle.Seconds()),
		})
	}

	return stats
}

func (c *Collector) collectPubSub(ctx context.Context) ([]*ChannelStat, error) {
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

	stats := make([]*ChannelStat, 0, len(names))
	for _, name := range names {
		stats = append(stats, &ChannelStat{
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
