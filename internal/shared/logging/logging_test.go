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

func (c *captured) last() Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.lines) == 0 {
		return Record{}
	}
	var record Record
	_ = json.Unmarshal([]byte(c.lines[len(c.lines)-1]), &record)
	return record
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
	if record.Attrs["agent"] != "nas-01" {
		t.Errorf("attrs[agent] = %v, want nas-01", record.Attrs["agent"])
	}
	if record.Attrs["scanner_slug"] != "movies" {
		t.Errorf("attrs[scanner_slug] = %v, want movies", record.Attrs["scanner_slug"])
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
	if record.Attrs["outer"] != "value" {
		t.Errorf("attrs[outer] = %v, want value (added before the group)", record.Attrs["outer"])
	}
	if record.Attrs["inner.key"] != "value" {
		t.Errorf("attrs[inner.key] = %v, want value (added after the group)", record.Attrs["inner.key"])
	}
}

// A WithAttrs/WithGroup clone must not mutate the handler it was derived
// from — two loggers sharing a parent must not see each other's attrs.
func TestWithAttrsDoesNotMutateTheOriginalHandler(t *testing.T) {
	logger, _, out := newTestLogger(t, 16)

	child := logger.With("child_only", "yes")
	logger.Info("from the parent")

	if attrs := out.last().Attrs; attrs["child_only"] != nil {
		t.Errorf("parent logger's record carried the child's attr: %v", attrs)
	}

	child.Info("from the child")
	if attrs := out.last().Attrs; attrs["child_only"] != "yes" {
		t.Errorf("child logger did not carry its own attr: %v", attrs)
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

	var record Record
	if err := json.Unmarshal(fake.messages[0], &record); err != nil {
		t.Fatalf("published message did not decode as a Record: %v", err)
	}
	if record.Message != "ships to the fake publisher" {
		t.Errorf("message = %q", record.Message)
	}
	if record.Source != "metarr-test" {
		t.Errorf("source = %q, want metarr-test", record.Source)
	}
}
