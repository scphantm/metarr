package busstats

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"Metarr/internal/shared/eventbus"
)

// collectStreams builds one BusStreamStat per durable stream the event bus
// knows about: the static stream-topic rows plus the per-agent command
// streams discovered against live Redis. Each Redis call is time-boxed on
// its own. A stream that cannot be read carries a per-stream error and does
// not cost the caller the rest of the snapshot; a reserved stream with no
// consumer group reads as present-but-not-created rather than as an error.
//
// It also returns the raw monotonic Redis counters this pass read: published
// maps a stream to its total entries-added, consumed maps groupSeriesKey to
// that group's total entries-read. recordStreamMetrics turns those into
// per-pass rates by subtracting the previous pass. A counter Redis does not
// report (miniredis omits entries-added; an uncreated stream has none) is
// simply absent from the map, which leaves that rate at zero.
func (s *Sampler) collectStreams(ctx context.Context) ([]*StreamStat, map[string]int64, map[string]int64) {
	// A partial SCAN failure still yields the streams it did find — someone
	// looking here to see why an agent is not picking up work should see
	// what there is rather than a blank panel.
	topics, _ := eventbus.DiscoverStreamTopics(ctx, s.client)

	stats := make([]*StreamStat, 0, len(topics))
	published := make(map[string]int64)
	consumed := make(map[string]int64)
	for _, topic := range topics {
		stats = append(stats, s.collectStream(ctx, topic, published, consumed))
	}
	return stats, published, consumed
}

func (s *Sampler) collectStream(ctx context.Context, topic eventbus.Topic, published, consumed map[string]int64) *StreamStat {
	stat := &StreamStat{
		Stream:    topic.Name,
		EventName: strings.Join(topic.Events, ", "),
		Groups:    []*GroupStat{},
	}

	// One XINFO STREAM serves both the depth and the entries-added counter
	// that feeds the publish rate. A stream key is created lazily when a
	// listener first subscribes, so "no such key" here is Redis's normal
	// cold-start answer, not a fault: show it as not-yet-created.
	infoCtx, cancel := context.WithTimeout(ctx, callTimeout)
	info, err := s.client.XInfoStream(infoCtx, topic.Name).Result()
	cancel()
	if err != nil {
		if !isMissingKey(err) {
			stat.Error = err.Error()
		}
		return stat
	}
	stat.Length = info.Length
	// entries-added is monotonic on real Redis; miniredis omits it and
	// go-redis then yields zero — a constant, so the derived publish rate
	// stays a sane zero there.
	published[topic.Name] = info.EntriesAdded

	groupsCtx, cancel := context.WithTimeout(ctx, callTimeout)
	groups, err := s.client.XInfoGroups(groupsCtx, topic.Name).Result()
	cancel()
	if err != nil {
		if isMissingKey(err) {
			return stat
		}
		stat.Error = err.Error()
		return stat
	}
	stat.Exists = true

	for _, group := range groups {
		stat.Groups = append(stat.Groups, s.collectGroup(ctx, topic.Name, group))
		// entries-read is a monotonic per-group counter. Redis always
		// reports it; miniredis returns null, which go-redis maps to zero —
		// a constant, so the derived consume rate stays a sane zero there.
		consumed[groupSeriesKey(topic.Name, group.Name)] = group.EntriesRead
	}
	return stat
}

// groupSeriesKey is the composite key a consumer group's per-metric history
// and previous-counter state are stored under. A group name is unique within
// its stream, so stream+group identifies it; the NUL separator cannot occur
// in a Redis key or group name.
func groupSeriesKey(stream, group string) string {
	return stream + "\x00" + group
}

func (s *Sampler) collectGroup(ctx context.Context, stream string, group redis.XInfoGroup) *GroupStat {
	lag := group.Lag
	if lag < 0 {
		// XINFO GROUPS reports -1 when it cannot compute lag (entries were
		// trimmed out from under the group). Zero reads better in the column
		// than a magic negative.
		lag = 0
	}

	stat := &GroupStat{
		Name:            group.Name,
		Consumers:       group.Consumers,
		Pending:         group.Pending,
		Lag:             lag,
		LastDeliveredId: group.LastDeliveredID,
		ConsumerDetail:  s.collectConsumers(ctx, stream, group.Name),
	}

	if group.Pending > 0 {
		stat.OldestPendingAgeSeconds = s.oldestPendingAge(ctx, stream, group.Name)
	}
	return stat
}

func (s *Sampler) collectConsumers(ctx context.Context, stream, group string) []*ConsumerStat {
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	consumers, err := s.client.XInfoConsumers(callCtx, stream, group).Result()
	cancel()
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

// oldestPendingAge returns the age in seconds of a group's oldest
// unacknowledged entry. A Redis Stream entry ID is "<ms>-<seq>", so the
// timestamp of the oldest pending entry is the millisecond prefix of the
// XPENDING summary's low ID. Returns zero when the age cannot be read.
func (s *Sampler) oldestPendingAge(ctx context.Context, stream, group string) int64 {
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	pending, err := s.client.XPending(callCtx, stream, group).Result()
	cancel()
	if err != nil || pending.Lower == "" {
		return 0
	}

	ms, ok := entryMillis(pending.Lower)
	if !ok {
		return 0
	}
	age := time.Since(time.UnixMilli(ms))
	if age < 0 {
		return 0
	}
	return int64(age.Seconds())
}

// entryMillis pulls the millisecond timestamp out of a Redis Stream entry ID
// ("1700000000000-0"). ok is false when id is not in that shape.
func entryMillis(id string) (int64, bool) {
	msText, _, found := strings.Cut(id, "-")
	if !found {
		msText = id
	}
	ms, err := strconv.ParseInt(msText, 10, 64)
	if err != nil {
		return 0, false
	}
	return ms, true
}

// isMissingKey reports whether err is Redis's "no such key" error, which
// XINFO returns for a stream that has not been created yet.
func isMissingKey(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no such key")
}
