// Package logging is the application-wide structured logger for both
// binaries. It writes every record to stdout synchronously and, in parallel,
// ships a copy to a centralized log pipeline (OpenObserve today, by way of
// Fluent Bit — see the package comment on Shipper) without ever letting that
// shipping add latency to the call site that logged.
//
// Two things are true of every record this package produces, and both are
// load-bearing for the centralized pipeline rather than incidental style:
//
//   - It always carries a "source" field — "metarr-server", or
//     "metarr-agent-<slug>" for an agent — so logs from every process land in
//     one stream and stay trivially filterable by which one emitted them.
//   - Dynamic data always arrives as a key-value attribute, never built into
//     the message string. See CLAUDE.md's Logging section: a flattened
//     message string is not searchable in OpenObserve the way a structured
//     field is, so this package's whole value proposition depends on callers
//     keeping that discipline — Handler doesn't and can't enforce it, since
//     slog has no way to inspect how a caller assembled its message string.
package logging

import (
	"log/slog"
	"os"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// defaultBufferSize is how many records may be queued for shipping before
// Handle starts dropping them. Sized generously — a burst of a few thousand
// log lines is normal; a shipper that is behind by that much is either about
// to catch up or genuinely down, and either way the app must not wait on it.
const defaultBufferSize = 4096

// Record is the vendor-neutral shape shipped over the wire — to Redis, and
// from there wherever Fluent Bit is configured to send it. It deliberately
// carries no vendor-specific field names (no OpenObserve "_timestamp", no
// Splunk "sourcetype"): Fluent Bit's own filters do that translation, which
// is what keeps the vendor swappable at the Fluent Bit layer instead of this
// one.
//
// It is an alias to the generated metarr.v1.LogRecord message — proto is the
// single definition for a model that crosses a language boundary, and this
// one crosses both the Redis channel and the wire to the Logging screen's
// live tail. See docs/adr/0005. The free-form attrs a caller attaches are
// carried as a *structpb.Struct so typing the record does not flatten them.
// The bytes on the Redis channel stay the flat JSON object they have always
// been — see the note in record.go on why the publish path does not go
// through the generated message to produce them.
type Record = metarrv1.LogRecord

// New returns a logger and the Shipper controlling where its records go.
// source identifies the emitting process — "metarr-server", or
// "metarr-agent-<slug>" for an agent — and is stamped on every record.
//
// The logger is fully usable the moment this returns: every record goes to
// stdout immediately, before any Redis connection exists. Call
// Shipper.Attach once a Redis client is available to start centralized
// shipping; nothing logged before that point is lost beyond the normal
// bounded-buffer limit described on Handle.
func New(source string) (*slog.Logger, *Shipper) {
	return NewWithBufferSize(source, defaultBufferSize)
}

// NewWithBufferSize is New with an explicit buffer capacity, for tests that
// need to observe backpressure without queueing thousands of records first.
func NewWithBufferSize(source string, bufferSize int) (*slog.Logger, *Shipper) {
	level := &slog.LevelVar{}
	level.Set(slog.LevelInfo)

	shipper := newShipper(bufferSize, level)

	handler := &Handler{
		source:  source,
		level:   level,
		stdout:  os.Stdout,
		buffer:  shipper.buffer,
		dropped: shipper.dropped,
	}

	return slog.New(handler), shipper
}
