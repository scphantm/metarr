package eventbus

import (
	"time"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// Retry policy defaults.
//
// These are the sane hardcoded values this slice ships with. The event_bus
// config section (docs/adr/0006) moves them to live configuration and feeds
// them in through a RetryPolicy value instead.
const (
	// DefaultRetryAttempts is the number of retries after the first attempt,
	// so a handler runs at most DefaultRetryAttempts+1 times before its
	// message is logged at error level and acked (dropped).
	DefaultRetryAttempts = 4
	// DefaultRetryBackoffBase is the wait before the first retry.
	DefaultRetryBackoffBase = 500 * time.Millisecond
	// DefaultRetryBackoffMax caps the exponential backoff between retries.
	DefaultRetryBackoffMax = 30 * time.Second

	// retryBackoffMultiplier is the factor each successive backoff is scaled
	// by. Not tunable: doubling is the conventional choice and the config
	// section deliberately exposes only base and max.
	retryBackoffMultiplier = 2.0
)

// RetryPolicy is the retry tuning the Bus applies to every durable-stream
// handler it registers. A message whose error survives every retry is logged
// and acked; there is no dead-letter stream. It is one slice of BusPolicy;
// DefaultBusPolicy and BusPolicyFromConfig build it.
type RetryPolicy struct {
	// MaxAttempts is the number of retries after the first attempt.
	MaxAttempts int
	// BackoffBase is the wait before the first retry.
	BackoffBase time.Duration
	// BackoffMax caps the exponential backoff.
	BackoffMax time.Duration
}

// BusPolicy is the single place every event-bus tuning knob is assembled: the
// publish/sweep retention policy, the durable-stream retry policy, and the
// retention sweep interval. Each bus constructor still takes only the
// sub-policy it uses (docs/adr/0006) — BusPolicy is the assembly point the
// two entrypoints build once, from live config on the server and from
// DefaultBusPolicy on the agent, then pass the sub-slices on.
type BusPolicy struct {
	// Retention is the size-cap and age-floor tuning the stream publisher and
	// the retention sweeper apply to every stream.
	Retention RetentionPolicy
	// Retry is the retry tuning the Bus applies to every durable-stream handler.
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
