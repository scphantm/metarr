package busstats

import (
	"context"
	"sort"

	"Metarr/internal/shared/eventbus"
)

// collectChannels builds one BusChannelStat per Pub/Sub channel: every channel
// live against Redis right now (PUBSUB CHANNELS) unioned with the fixed
// known-channel list, so a declared channel with no current subscriber still
// shows as a row rather than vanishing. Each channel's subscriber count comes
// from a single PUBSUB NUMSUB over the union.
//
// There is deliberately no depth and no publish rate: Redis Pub/Sub buffers
// nothing and exposes no per-channel publish counter. A row's `known` flag is
// true for a declared channel and false for one discovered at runtime — the
// per-correlation-id reply channels — so the dashboard can tell a transient
// channel from a declared one and flag a declared channel that has gone dead.
//
// Each Redis call is time-boxed on its own. A failed PUBSUB CHANNELS still
// yields the known channels; a failed PUBSUB NUMSUB still yields the rows at a
// zero count. One unreadable call does not cost the caller the rest of the
// snapshot.
func (s *Sampler) collectChannels(ctx context.Context) []*ChannelStat {
	known := eventbus.KnownPubSubChannels()
	knownSet := make(map[string]struct{}, len(known))
	for _, name := range known {
		knownSet[name] = struct{}{}
	}

	names := make(map[string]struct{}, len(known))
	for _, name := range known {
		names[name] = struct{}{}
	}

	listCtx, cancelList := context.WithTimeout(ctx, callTimeout)
	live, err := s.client.PubSubChannels(listCtx, "*").Result()
	cancelList()
	if err == nil {
		for _, name := range live {
			names[name] = struct{}{}
		}
	}

	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	// Declared channels first, then alphabetical within each group, so the
	// table order is stable across passes regardless of Redis's iteration
	// order and the reply.* channels sort below the fixed ones.
	sort.Slice(ordered, func(i, j int) bool {
		_, iKnown := knownSet[ordered[i]]
		_, jKnown := knownSet[ordered[j]]
		if iKnown != jKnown {
			return iKnown
		}
		return ordered[i] < ordered[j]
	})

	counts := map[string]int64{}
	if len(ordered) > 0 {
		countCtx, cancelCount := context.WithTimeout(ctx, callTimeout)
		got, err := s.client.PubSubNumSub(countCtx, ordered...).Result()
		cancelCount()
		if err == nil {
			counts = got
		}
	}

	stats := make([]*ChannelStat, 0, len(ordered))
	for _, name := range ordered {
		_, isKnown := knownSet[name]
		stats = append(stats, &ChannelStat{
			Channel:     name,
			Subscribers: counts[name],
			Known:       isKnown,
		})
	}
	return stats
}
