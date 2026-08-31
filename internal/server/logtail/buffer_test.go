package logtail

import (
	"encoding/json"
	"testing"
)

// line builds one record in the flat JSON form published to
// eventbus.LogChannel — the same shape internal/shared/logging.marshalLogLine
// writes — so the buffer is exercised against realistic bytes.
func line(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encoding fixture record: %v", err)
	}
	return data
}

func msg(t *testing.T, message string) []byte {
	t.Helper()
	return line(t, map[string]any{
		"time":    "2026-08-31T00:00:00Z",
		"level":   "INFO",
		"message": message,
		"source":  "metarr-server",
	})
}

func TestRecentReturnsWhatWasAdded(t *testing.T) {
	buffer := NewBuffer(10)
	buffer.Add(msg(t, "one"))
	buffer.Add(msg(t, "two"))

	recent := buffer.Recent()
	if len(recent) != 2 {
		t.Fatalf("got %d records, want 2", len(recent))
	}
	if recent[0].Message != "one" || recent[1].Message != "two" {
		t.Errorf("order = %+v, want [one, two]", recent)
	}
}

// The buffer exists to bound memory, so it must actually evict rather than
// grow forever.
func TestBufferEvictsOldestWhenFull(t *testing.T) {
	buffer := NewBuffer(3)
	for i := range 5 {
		buffer.Add(msg(t, string(rune('a'+i))))
	}

	recent := buffer.Recent()
	if len(recent) != 3 {
		t.Fatalf("got %d records, want 3", len(recent))
	}
	// The first two ("a", "b") should have been evicted, leaving c, d, e.
	got := []string{recent[0].Message, recent[1].Message, recent[2].Message}
	want := []string{"c", "d", "e"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("recent = %v, want %v", got, want)
			break
		}
	}
}

// A record that fails to decode — a malformed publish, a version mismatch —
// must not corrupt or wedge the buffer for records around it.
func TestAddIgnoresUndecodableRecords(t *testing.T) {
	buffer := NewBuffer(10)
	buffer.Add(msg(t, "before"))
	buffer.Add([]byte("not json"))
	buffer.Add(msg(t, "after"))

	recent := buffer.Recent()
	if len(recent) != 2 {
		t.Fatalf("got %d records, want 2 (the malformed one dropped): %+v", len(recent), recent)
	}
	if recent[0].Message != "before" || recent[1].Message != "after" {
		t.Errorf("recent = %+v", recent)
	}
}

// A record's free-form attrs are structured data, not an opaque blob: they
// must come back off the buffer with their shape intact so the tail can be
// rendered against a real type.
func TestAddPreservesStructuredAttrs(t *testing.T) {
	buffer := NewBuffer(10)
	buffer.Add(line(t, map[string]any{
		"time":    "2026-08-31T00:00:00Z",
		"level":   "INFO",
		"message": "scan done",
		"source":  "metarr-server",
		"attrs": map[string]any{
			"agent":  "nas-01",
			"counts": map[string]any{"added": 2},
		},
	}))

	fields := buffer.Recent()[0].GetAttrs().GetFields()
	if got := fields["agent"].GetStringValue(); got != "nas-01" {
		t.Errorf("attrs.agent = %q, want nas-01", got)
	}
	if got := fields["counts"].GetStructValue().GetFields()["added"].GetNumberValue(); got != 2 {
		t.Errorf("attrs.counts.added = %v, want 2", got)
	}
}

// Recent hands back a fresh slice: adding more records after the call must
// not disturb what an earlier caller received.
func TestRecentReturnsASliceIndependentOfLaterAdds(t *testing.T) {
	buffer := NewBuffer(10)
	buffer.Add(msg(t, "one"))

	snapshot := buffer.Recent()
	buffer.Add(msg(t, "two"))

	if len(snapshot) != 1 || snapshot[0].Message != "one" {
		t.Errorf("earlier snapshot changed after a later Add: %+v", snapshot)
	}
}

func TestNewBufferStartsEmpty(t *testing.T) {
	if got := NewBuffer(10).Recent(); len(got) != 0 {
		t.Errorf("got %d records, want 0", len(got))
	}
}
