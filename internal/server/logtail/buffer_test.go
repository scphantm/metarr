package logtail

import (
	"encoding/json"
	"testing"

	"Metarr/internal/shared/logging"
)

func encode(t *testing.T, record logging.Record) []byte {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("encoding fixture record: %v", err)
	}
	return data
}

func TestRecentReturnsWhatWasAdded(t *testing.T) {
	buffer := NewBuffer(10)
	buffer.Add(encode(t, logging.Record{Message: "one", Source: "metarr-server"}))
	buffer.Add(encode(t, logging.Record{Message: "two", Source: "metarr-server"}))

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
		buffer.Add(encode(t, logging.Record{Message: string(rune('a' + i))}))
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
	buffer.Add(encode(t, logging.Record{Message: "before"}))
	buffer.Add([]byte("not json"))
	buffer.Add(encode(t, logging.Record{Message: "after"}))

	recent := buffer.Recent()
	if len(recent) != 2 {
		t.Fatalf("got %d records, want 2 (the malformed one dropped): %+v", len(recent), recent)
	}
	if recent[0].Message != "before" || recent[1].Message != "after" {
		t.Errorf("recent = %+v", recent)
	}
}

func TestRecentReturnsACopyNotTheLiveBuffer(t *testing.T) {
	buffer := NewBuffer(10)
	buffer.Add(encode(t, logging.Record{Message: "one"}))

	snapshot := buffer.Recent()
	snapshot[0].Message = "mutated"

	if got := buffer.Recent()[0].Message; got != "one" {
		t.Errorf("mutating a snapshot changed the buffer: %q", got)
	}
}

func TestNewBufferStartsEmpty(t *testing.T) {
	if got := NewBuffer(10).Recent(); len(got) != 0 {
		t.Errorf("got %d records, want 0", len(got))
	}
}
