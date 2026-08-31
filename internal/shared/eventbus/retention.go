package eventbus

import (
	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// Stream retention defaults.
//
// Sane hardcoded values this slice ships with. The event_bus config section
// (docs/adr/0006) moves them to live configuration and feeds them in through
// a RetentionPolicy value; nothing else about publishing or the sweep
// changes when that happens.
const (
	// DefaultMaxLen is the approximate entry cap applied to every stream at
	// publish time.
	DefaultMaxLen = 10_000
	// DefaultRetentionHours is the age floor the trim sweep enforces, and the
	// history an external subscriber can rely on: at least this many hours,
	// or MAXLEN entries, whichever is larger. A consumer offline longer
	// reconciles through the query APIs.
	DefaultRetentionHours = 48
)

// RetentionPolicy is the size-cap and age-floor tuning applied to every
// stream: one cap at publish time, age at sweep time.
type RetentionPolicy struct {
	// MaxLen is the approximate cap applied to every stream.
	MaxLen int64
	// RetentionHours is how far back the trim sweep keeps entries.
	RetentionHours int
}

// DefaultRetentionPolicy returns the built-in policy. It is the reference
// the event_bus section's built-in defaults (builtin_defaults.json) must
// agree with, and what a process with no live config to read — the agent —
// runs with.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		MaxLen:         DefaultMaxLen,
		RetentionHours: DefaultRetentionHours,
	}
}

// RetentionPolicyFromConfig builds a RetentionPolicy from the live event_bus
// config section. The server reads its publish cap and sweep window this
// way instead of from the constants above (docs/adr/0006).
func RetentionPolicyFromConfig(c *metarrv1.EventBusConfig) RetentionPolicy {
	return RetentionPolicy{
		MaxLen:         int64(c.GetMaxLen()),
		RetentionHours: int(c.GetRetentionHours()),
	}
}
