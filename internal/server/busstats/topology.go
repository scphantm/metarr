package busstats

import (
	"context"
	"slices"
	"sort"

	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/eventbus"
)

// ExpectedStream is one durable stream the topology says should exist and the
// identities that should be reading its consumer group. Identities is empty
// for a reserved stream nothing consumes yet — such a row is never flagged.
type ExpectedStream struct {
	Stream     string
	Group      string
	Identities []string
}

// ExpectedChannel is one Pub/Sub channel the topology says should have a
// subscriber, and the identities expected to be attached.
type ExpectedChannel struct {
	Channel    string
	Identities []string
}

// Topology is the expected-vs-actual reference: every stream and channel the
// bus should carry given the durable stream-topic list and the set of
// registered agents, and per row the identities that should be attached. It
// is a pure function of its inputs (DeriveTopology) — no Redis — so the
// derivation is table-testable on its own.
type Topology struct {
	Streams  map[string]ExpectedStream
	Channels map[string]ExpectedChannel
}

// DeriveTopology builds the expected topology from the durable stream-topic
// list and the registered agent slugs. It is pure: given the same inputs it
// returns the same rows, and it touches no Redis.
//
//   - A static stream that a listener consumes expects metarr-server on its
//     group; a reserved stream with no group expects nothing.
//   - The fixed known Pub/Sub channels expect metarr-server.
//   - Every registered agent expects metarr-agent-<slug> on its command
//     stream group and on each of its two per-agent Pub/Sub channels, whether
//     or not that agent is online right now.
//
// Pattern rows in topics are skipped: the per-agent command streams come from
// slugs here, not from expanding a glob against Redis.
func DeriveTopology(topics []eventbus.Topic, slugs []string) Topology {
	top := Topology{
		Streams:  make(map[string]ExpectedStream),
		Channels: make(map[string]ExpectedChannel),
	}

	for _, topic := range topics {
		if topic.Pattern {
			continue
		}
		expected := ExpectedStream{Stream: topic.Name, Group: topic.Group}
		if topic.Consumed && topic.Group != "" {
			expected.Identities = []string{eventbus.SourceServer}
		}
		top.Streams[topic.Name] = expected
	}

	for _, channel := range eventbus.KnownPubSubChannels() {
		top.Channels[channel] = ExpectedChannel{
			Channel:    channel,
			Identities: []string{eventbus.SourceServer},
		}
	}

	for _, slug := range slugs {
		agent := eventbus.AgentSource(slug)

		command := eventbus.AgentCommandTopic(slug)
		top.Streams[command.Name] = ExpectedStream{
			Stream:     command.Name,
			Group:      command.Group,
			Identities: []string{agent},
		}

		for _, channel := range eventbus.AgentPubSubChannels(slug) {
			top.Channels[channel] = ExpectedChannel{
				Channel:    channel,
				Identities: []string{agent},
			}
		}
	}

	return top
}

// nameMissing attributes a shortfall on a flagged row to specific identities.
// An agent identity is named when its slug has no live presence key, or when
// countShort says the live count is below the expected count regardless of
// presence (the agent is here but not attached). The server identity is named
// only when countShort — this process cannot presence-check itself, so on an
// otherwise-healthy row it is assumed attached. The result can be empty.
func nameMissing(expected []string, present map[string]bool, countShort bool) []string {
	var missing []string
	for _, identity := range expected {
		if slug, ok := eventbus.SlugFromAgentSource(identity); ok {
			if !present[slug] || countShort {
				missing = append(missing, identity)
			}
			continue
		}
		if countShort {
			missing = append(missing, identity)
		}
	}
	return missing
}

// applyStreamTopology annotates the sampled stream rows with the expected
// topology and unions in the rows Redis never surfaced. An offline agent's
// command stream has no Redis key, so discovery misses it; it is added here
// as a not-created row so the agent shows as broken rather than absent.
//
// A row is flagged when a registered agent it expects has no live presence
// key, or when the group it expects exists in Redis but has fewer consumers
// than expected. A group that has not been created yet is left alone — durable
// streams are created lazily, so a server-consumed stream nobody has read on a
// fresh start is cold-start, not a fault (docs/adr/0007). A reserved stream
// (no expected identities) is never flagged. Rows are returned sorted by name
// so the table order is stable across passes.
func applyStreamTopology(streams []*StreamStat, top Topology, present map[string]bool) []*StreamStat {
	byName := make(map[string]*StreamStat, len(streams))
	for _, stat := range streams {
		byName[stat.Stream] = stat
	}

	for name := range top.Streams {
		if _, ok := byName[name]; ok {
			continue
		}
		stat := &StreamStat{Stream: name, Groups: []*GroupStat{}}
		byName[name] = stat
		streams = append(streams, stat)
	}

	for _, stat := range streams {
		expected, ok := top.Streams[stat.Stream]
		if !ok {
			continue
		}
		stat.ExpectedIdentities = slices.Clone(expected.Identities)
		if len(expected.Identities) == 0 {
			continue
		}

		groupExists := false
		var consumers int64
		for _, group := range stat.Groups {
			if group.Name == expected.Group {
				groupExists, consumers = true, group.Consumers
			}
		}
		underConsumed := groupExists && consumers < int64(len(expected.Identities))

		missing := nameMissing(expected.Identities, present, underConsumed)
		if len(missing) > 0 {
			stat.Flagged = true
			stat.MissingIdentities = missing
		}
	}

	sort.Slice(streams, func(i, j int) bool { return streams[i].Stream < streams[j].Stream })
	return streams
}

// applyChannelTopology does for the Pub/Sub channel rows what
// applyStreamTopology does for streams: it unions in every expected channel
// Redis did not surface (an offline agent's config-changed and request
// channels have no live subscriber), marks every topology channel known, and
// flags a row whose subscriber count is below its expected identity count.
func applyChannelTopology(channels []*ChannelStat, top Topology, present map[string]bool) []*ChannelStat {
	byName := make(map[string]*ChannelStat, len(channels))
	for _, stat := range channels {
		byName[stat.Channel] = stat
	}

	for name := range top.Channels {
		if _, ok := byName[name]; ok {
			continue
		}
		stat := &ChannelStat{Channel: name, Known: true}
		byName[name] = stat
		channels = append(channels, stat)
	}

	for _, stat := range channels {
		expected, ok := top.Channels[stat.Channel]
		if !ok {
			continue
		}
		stat.Known = true
		stat.ExpectedIdentities = slices.Clone(expected.Identities)

		if stat.Subscribers < int64(len(expected.Identities)) {
			stat.Flagged = true
			stat.MissingIdentities = nameMissing(expected.Identities, present, true)
		}
	}

	sortChannelStats(channels)
	return channels
}

// collectPresence returns the set of agent slugs with a live presence key,
// scanned straight off Redis. It is the same key family agentregistry reads,
// used here only to say which expected agent identity is the one missing from
// a flagged row. A failed or timed-out scan yields an empty set — the row is
// still flagged on the count, it just cannot name the identity.
func (s *Sampler) collectPresence(ctx context.Context) map[string]bool {
	present := map[string]bool{}

	scanCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	iterator := s.client.Scan(scanCtx, 0, agentproto.PresenceKeyPattern, 100).Iterator()
	for iterator.Next(scanCtx) {
		if slug := agentproto.SlugFromPresenceKey(iterator.Val()); slug != "" {
			present[slug] = true
		}
	}
	return present
}
