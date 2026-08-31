package eventbus

import (
	"time"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// BusPolicy is the single place every event-bus tuning knob is assembled: the
// publish/sweep retention policy, the router retry policy, and the retention
// sweep interval. Each bus constructor still takes only the sub-policy it
// uses (docs/adr/0006) — BusPolicy is the assembly point the two entrypoints
// build once, from live config on the server and from DefaultBusPolicy on the
// agent, then pass the sub-slices on.
type BusPolicy struct {
	// Retention is the size-cap and age-floor tuning the stream publisher and
	// the retention sweeper apply to every stream.
	Retention RetentionPolicy
	// Retry is the retry tuning the Router applies to every handler.
	Retry RetryPolicy
	// SweepInterval is how often the retention sweep runs. It stays a compiled
	// default; it is deliberately not a live event_bus config field.
	SweepInterval time.Duration
}

// DefaultBusPolicy returns the built-in bus tuning. It is the sole public
// default constructor: the reference the event_bus built-in defaults
// (builtin_defaults.json) must agree with, and what a process with no live
// config to read — the agent — runs with.
func DefaultBusPolicy() BusPolicy {
	return BusPolicy{
		Retention: RetentionPolicy{
			MaxLen:         DefaultMaxLen,
			RetentionHours: DefaultRetentionHours,
		},
		Retry: RetryPolicy{
			MaxAttempts: DefaultRetryAttempts,
			BackoffBase: DefaultRetryBackoffBase,
			BackoffMax:  DefaultRetryBackoffMax,
		},
		SweepInterval: DefaultSweepInterval,
	}
}

// BusPolicyFromConfig builds a whole BusPolicy from the live event_bus config
// section (docs/adr/0006). It is the only place the generated EventBusConfig
// type appears in the eventbus package's public interface. The sweep interval
// is not a live config field, so it stays the compiled DefaultSweepInterval.
func BusPolicyFromConfig(c *metarrv1.EventBusConfig) BusPolicy {
	return BusPolicy{
		Retention: RetentionPolicy{
			MaxLen:         int64(c.GetMaxLen()),
			RetentionHours: int(c.GetRetentionHours()),
		},
		Retry: RetryPolicy{
			MaxAttempts: int(c.GetRetryAttempts()),
			BackoffBase: time.Duration(c.GetRetryBackoffBaseMs()) * time.Millisecond,
			BackoffMax:  time.Duration(c.GetRetryBackoffMaxMs()) * time.Millisecond,
		},
		SweepInterval: DefaultSweepInterval,
	}
}
