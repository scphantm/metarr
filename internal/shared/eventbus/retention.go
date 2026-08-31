package eventbus

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
// stream: one cap at publish time, age at sweep time. It is one slice of
// BusPolicy; DefaultBusPolicy and BusPolicyFromConfig build it.
type RetentionPolicy struct {
	// MaxLen is the approximate cap applied to every stream.
	MaxLen int64
	// RetentionHours is how far back the trim sweep keeps entries.
	RetentionHours int
}
