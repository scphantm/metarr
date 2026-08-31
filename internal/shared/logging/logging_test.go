package logging

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// captured is an io.Writer that keeps every line written to it, for asserting
// on exactly what Handle sent to stdout.
type captured struct {
	mu    sync.Mutex
	lines []string
}

func (c *captured) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func (c *captured) last() *Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.lines) == 0 {
		return &Record{}
	}
	record, err := UnmarshalRecord([]byte(c.lines[len(c.lines)-1]))
	if err != nil {
		return &Record{}
	}
	return record
}

// strAttr reads one flattened attribute off a decoded record. Every attr the
// tests here attach is a string, so a plain accessor keeps the assertions
// readable; a missing key comes back "".
func strAttr(record *Record, key string) string {
	return record.GetAttrs().GetFields()[key].GetStringValue()
}

// hasAttr reports whether the decoded record carries an attribute under key
// at all, regardless of its value.
func hasAttr(record *Record, key string) bool {
	_, ok := record.GetAttrs().GetFields()[key]
	return ok
}

// rawLast returns the exact bytes of the most recent line written to stdout,
// for assertions about the on-the-wire JSON shape rather than a decoded
// record.
func (c *captured) rawLast(t *testing.T) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.lines) == 0 {
		t.Fatal("no record was written")
	}
	return c.lines[len(c.lines)-1]
}

func (c *captured) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.lines)
}

// newTestLogger builds a logger the same way New does, but swaps in a
// captured stdout writer so tests can inspect what was written without
// scraping the real os.Stdout.
func newTestLogger(t *testing.T, bufferSize int) (*slog.Logger, *Shipper, *captured) {
	t.Helper()

	logger, shipper := NewWithBufferSize("metarr-test", bufferSize)
	handler := logger.Handler().(*Handler)

	out := &captured{}
	handler.stdout = out

	return logger, shipper, out
}

// The bytes on the Redis channel are what Fluent Bit consumes verbatim, so
// the record must serialize as the same flat JSON object it always has:
// snake_case top-level keys, and no attrs key at all when the caller
// attached nothing.
func TestRecordSerializesAsFlatVendorNeutralJSON(t *testing.T) {
	logger, _, out := newTestLogger(t, 16)

	logger.Info("no attrs here")

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out.rawLast(t)), &decoded); err != nil {
		t.Fatalf("record was not a JSON object: %v", err)
	}
	for _, key := range []string{"time", "level", "message", "source"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("record is missing top-level key %q: %v", key, decoded)
		}
	}
	if _, ok := decoded["attrs"]; ok {
		t.Errorf("record carried an attrs key with nothing attached: %v", decoded)
	}

	logger.With("agent", "nas-01").Info("with an attr")
	if err := json.Unmarshal([]byte(out.rawLast(t)), &decoded); err != nil {
		t.Fatalf("record was not a JSON object: %v", err)
	}
	if _, ok := decoded["attrs"]; !ok {
		t.Errorf("record dropped the attrs the caller attached: %v", decoded)
	}
}

// protojson would drop an empty string field; the flat JSON on the Redis
// channel must keep message present even when it is "" so a downstream
// field-presence filter still sees it, exactly as the old struct did.
func TestAnEmptyMessageStillEmitsTheKey(t *testing.T) {
	logger, _, out := newTestLogger(t, 16)

	logger.Info("")

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out.rawLast(t)), &decoded); err != nil {
		t.Fatalf("record was not a JSON object: %v", err)
	}
	if got, ok := decoded["message"]; !ok || string(got) != `""` {
		t.Errorf("message key = %s, present = %v, want present and empty", got, ok)
	}
}

// An integer attribute past 2^53 must reach the Redis channel byte-exact.
// google.protobuf.Struct holds numbers as float64, so the publish path
// serialises the raw attribute map rather than the generated message — this
// pins that it stays that way.
func TestLargeIntegerAttrKeepsFullPrecisionOnTheWire(t *testing.T) {
	logger, _, out := newTestLogger(t, 16)

	const nanos int64 = 1725060645123456789 // ~2024, well past 2^53
	logger.Info("scan finished", "observed_at", nanos)

	if raw := out.rawLast(t); !strings.Contains(raw, "1725060645123456789") {
		t.Errorf("wire record lost integer precision: %s", raw)
	}
}

// Every record must carry the source this logger was constructed with — the
// one field the whole centralized pipeline depends on for filtering which
// process a line came from.
func TestEveryRecordCarriesItsSource(t *testing.T) {
	logger, _, out := newTestLogger(t, 16)

	logger.Info("agent starting")
	logger.With("scan_id", "abc").Warn("scan started")

	if got := out.last().Source; got != "metarr-test" {
		t.Errorf("source = %q, want metarr-test", got)
	}
}

// The defining property of this package: Handle must never block, no matter
// how far behind the shipper is. A tiny buffer with nothing draining it is
// the worst case, and even there, logging must return immediately.
func TestHandleNeverBlocksWhenTheBufferIsFull(t *testing.T) {
	logger, _, _ := newTestLogger(t, 2)

	done := make(chan struct{})
	go func() {
		// Nothing ever attaches a shipper here, so nothing drains the buffer —
		// this is deliberately the worst case.
		for i := 0; i < 1000; i++ {
			logger.Info("filling the buffer", "i", i)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Handle blocked instead of dropping once the buffer filled")
	}
}

// A dropped record must be counted, not silently discarded — that's what
// lets the periodic self-report say something happened.
func TestOverflowIncrementsTheDroppedCounter(t *testing.T) {
	logger, shipper, _ := newTestLogger(t, 1)

	for i := 0; i < 50; i++ {
		logger.Info("more than the buffer can hold")
	}

	if got := shipper.dropped.Load(); got <= 0 {
		t.Errorf("dropped count = %d, want > 0", got)
	}
}

// Level is a live knob: SetLevel must change what Enabled allows through on
// the very next call, with no reconstruction of the logger.
func TestSetLevelTakesEffectImmediately(t *testing.T) {
	logger, shipper, out := newTestLogger(t, 16)

	logger.Debug("before raising verbosity")
	if got := out.count(); got != 0 {
		t.Fatalf("debug line was emitted at Info level: %d lines written", got)
	}

	shipper.SetLevel(slog.LevelDebug)
	logger.Debug("after raising verbosity")
	if got := out.count(); got != 1 {
		t.Fatalf("debug line was not emitted after SetLevel(Debug): %d lines written", got)
	}

	shipper.SetLevel(slog.LevelInfo)
	logger.Debug("after lowering verbosity again")
	if got := out.count(); got != 1 {
		t.Fatalf("debug line was emitted after SetLevel(Info): %d lines written", got)
	}
}

// With(...) chains — used throughout both binaries — must show up as
// attributes on the shipped/logged record, not just be silently accepted.
func TestWithAttrsArePreservedOnTheRecord(t *testing.T) {
	logger, _, out := newTestLogger(t, 16)

	logger.With("agent", "nas-01", "scanner_slug", "movies").Info("scan started")

	record := out.last()
	if got := strAttr(record, "agent"); got != "nas-01" {
		t.Errorf("attrs[agent] = %q, want nas-01", got)
	}
	if got := strAttr(record, "scanner_slug"); got != "movies" {
		t.Errorf("attrs[scanner_slug] = %q, want movies", got)
	}
}

// A record's attrs are carried as a structured value, not a flattened
// string: a non-scalar attribute value must survive the encode/decode with
// its shape intact. This is the property the generated-message slice exists
// to add — see docs/adr/0005.
func TestStructuredAttrValuesSurviveTheRoundTrip(t *testing.T) {
	logger, _, out := newTestLogger(t, 16)

	logger.Info("scan finished",
		"counts", map[string]any{"added": 3, "skipped": 1},
		"paths", []any{"/a", "/b"},
	)

	fields := out.last().GetAttrs().GetFields()

	counts := fields["counts"].GetStructValue().GetFields()
	if got := counts["added"].GetNumberValue(); got != 3 {
		t.Errorf("attrs.counts.added = %v, want 3", got)
	}
	if got := counts["skipped"].GetNumberValue(); got != 1 {
		t.Errorf("attrs.counts.skipped = %v, want 1", got)
	}

	paths := fields["paths"].GetListValue().GetValues()
	if len(paths) != 2 || paths[0].GetStringValue() != "/a" || paths[1].GetStringValue() != "/b" {
		t.Errorf("attrs.paths = %v, want [/a /b]", paths)
	}
}

// Attrs added via With() before a WithGroup call must keep their original,
// unprefixed keys — only attrs added after belong to the group.
func TestWithGroupOnlyAffectsAttrsAddedAfterIt(t *testing.T) {
	logger, _, out := newTestLogger(t, 16)

	logger = logger.With("outer", "value")
	logger = slog.New(logger.Handler().WithGroup("inner"))
	logger.Info("grouped", "key", "value")

	record := out.last()
	if got := strAttr(record, "outer"); got != "value" {
		t.Errorf("attrs[outer] = %q, want value (added before the group)", got)
	}
	if got := strAttr(record, "inner.key"); got != "value" {
		t.Errorf("attrs[inner.key] = %q, want value (added after the group)", got)
	}
}

// A WithAttrs/WithGroup clone must not mutate the handler it was derived
// from — two loggers sharing a parent must not see each other's attrs.
func TestWithAttrsDoesNotMutateTheOriginalHandler(t *testing.T) {
	logger, _, out := newTestLogger(t, 16)

	child := logger.With("child_only", "yes")
	logger.Info("from the parent")

	if record := out.last(); hasAttr(record, "child_only") {
		t.Errorf("parent logger's record carried the child's attr: %v", record.GetAttrs())
	}

	child.Info("from the child")
	if got := strAttr(out.last(), "child_only"); got != "yes" {
		t.Errorf("child logger did not carry its own attr: %q", got)
	}
}

// fakePublisher is the two-line Publisher fake the package comment on
// Publisher promises is possible instead of standing up real Redis.
type fakePublisher struct {
	mu       sync.Mutex
	messages [][]byte
	received chan struct{}
}

func newFakePublisher() *fakePublisher {
	return &fakePublisher{received: make(chan struct{}, 1024)}
}

// Publish satisfies logging.Publisher. Its whole point is being this small —
// a real Redis client's Publish is a network round trip; this is two lines.
func (f *fakePublisher) Publish(ctx context.Context, channel string, message any) *redis.IntCmd {
	f.mu.Lock()
	f.messages = append(f.messages, append([]byte(nil), message.([]byte)...))
	f.mu.Unlock()

	f.received <- struct{}{}

	cmd := redis.NewIntCmd(ctx, "publish", channel, message)
	cmd.SetVal(1)
	return cmd
}

func TestShipperPublishesBufferedRecords(t *testing.T) {
	logger, shipper, _ := newTestLogger(t, 16)

	fake := newFakePublisher()
	shipper.Attach(fake)

	logger.Info("ships to the fake publisher")

	select {
	case <-fake.received:
	case <-time.After(2 * time.Second):
		t.Fatal("shipper never published the buffered record")
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.messages) != 1 {
		t.Fatalf("fake publisher received %d messages, want 1", len(fake.messages))
	}

	record, err := UnmarshalRecord(fake.messages[0])
	if err != nil {
		t.Fatalf("published message did not decode as a Record: %v", err)
	}
	if record.Message != "ships to the fake publisher" {
		t.Errorf("message = %q", record.Message)
	}
	if record.Source != "metarr-test" {
		t.Errorf("source = %q, want metarr-test", record.Source)
	}
}
