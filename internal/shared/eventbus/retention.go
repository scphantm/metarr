package eventbus

import (
	"context"

	"github.com/redis/go-redis/v9"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// Stream retention defaults.
//
// Sane hardcoded values this slice ships with. The event_bus config section
// (docs/adr/0006) moves them to live configuration and feeds them in through
// a RetentionPolicy value; nothing else about publishing or the sweep
// changes when that happens.
const (
	// DefaultMaxLenHigh is the approximate entry cap for the high-volume
	// result streams (HighVolumeStreams).
	DefaultMaxLenHigh = 100_000
	// DefaultMaxLenDefault is the approximate entry cap for every other
	// stream, the dead-letter stream included.
	DefaultMaxLenDefault = 10_000
	// DefaultRetentionHours is the age floor the trim sweep enforces, and the
	// history an external subscriber can rely on: at least this many hours,
	// or MAXLEN entries, whichever is larger. A consumer offline longer
	// reconciles through the query APIs.
	DefaultRetentionHours = 48
)

// RetentionPolicy is the size-cap and age-floor tuning applied to every
// stream: caps at publish time, age at sweep time.
type RetentionPolicy struct {
	// MaxLenHigh is the approximate cap for HighVolumeStreams.
	MaxLenHigh int64
	// MaxLenDefault is the approximate cap for every other stream.
	MaxLenDefault int64
	// RetentionHours is how far back the trim sweep keeps entries.
	RetentionHours int
}

// DefaultRetentionPolicy returns the built-in policy. It is the reference
// the event_bus section's built-in defaults (builtin_defaults.json) must
// agree with, and what a process with no live config to read — the agent —
// runs with.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		MaxLenHigh:     DefaultMaxLenHigh,
		MaxLenDefault:  DefaultMaxLenDefault,
		RetentionHours: DefaultRetentionHours,
	}
}

// RetentionPolicyFromConfig builds a RetentionPolicy from the live event_bus
// config section. The server reads its publish caps and sweep window this
// way instead of from the constants above (docs/adr/0006).
func RetentionPolicyFromConfig(c *metarrv1.EventBusConfig) RetentionPolicy {
	return RetentionPolicy{
		MaxLenHigh:     int64(c.GetMaxLenHigh()),
		MaxLenDefault:  int64(c.GetMaxLenDefault()),
		RetentionHours: int(c.GetRetentionHours()),
	}
}

// HighVolumeStreams are capped at MaxLenHigh rather than MaxLenDefault: the
// agent result streams, which carry one entry per scanned item or executed
// workflow node.
func HighVolumeStreams() []string {
	return []string{AgentScanResultStream, AgentNodeResultStream}
}

// Maxlens is the per-stream approximate-cap map for a redisstream Publisher:
// HighVolumeStreams at MaxLenHigh, with MaxLenDefault covering the rest
// through the publisher's DefaultMaxlen.
func (p RetentionPolicy) Maxlens() map[string]int64 {
	high := HighVolumeStreams()
	caps := make(map[string]int64, len(high))
	for _, stream := range high {
		caps[stream] = p.MaxLenHigh
	}
	return caps
}

// sweepStreamNames returns every stream the retention sweep should trim by
// age: the fixed streams, the reserved node-result stream, the dead-letter
// stream, and every per-agent command stream currently in Redis.
func sweepStreamNames(ctx context.Context, client redis.UniversalClient) []string {
	seen := map[string]bool{}
	var names []string
	add := func(stream string) {
		if stream == "" || seen[stream] {
			return
		}
		seen[stream] = true
		names = append(names, stream)
	}

	for _, topic := range KnownStreams() {
		add(topic.Stream)
	}
	add(AgentNodeResultStream)
	add(DeadLetterStream)

	for _, pattern := range KnownStreamPatterns() {
		iterator := client.Scan(ctx, 0, pattern, 100).Iterator()
		for iterator.Next(ctx) {
			add(iterator.Val())
		}
		// A failed scan drops the dynamic streams from this pass rather than
		// the whole sweep; the fixed streams are still trimmed.
		_ = iterator.Err()
	}

	return names
}
