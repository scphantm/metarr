package scanmodel

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestMarshalStoredKeepsProtoNamesAndOmitsEmpty pins the two properties the
// Mongo indexes and the rebuildable-cache behaviour depend on: field names stay
// snake_case, and an unset field is absent rather than a spelled-out zero.
func TestMarshalStoredKeepsProtoNamesAndOmitsEmpty(t *testing.T) {
	encoded, err := MarshalStored(&MediaFile{
		RecordType:   RecordTypeMediaFile,
		Path:         "/media/Shows/S/ep.mkv",
		RelativePath: "ep.mkv",
	})
	if err != nil {
		t.Fatalf("MarshalStored() error = %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decoding MarshalStored output: %v", err)
	}

	if _, ok := fields["record_type"]; !ok {
		t.Errorf("expected snake_case key record_type, got keys %v", keysOf(fields))
	}
	for _, empty := range []string{"directory_id", "size_bytes", "scanned_at", "warnings", "sidecars"} {
		if _, present := fields[empty]; present {
			t.Errorf("unset field %q was emitted: %s", empty, encoded)
		}
	}
}

// TestNormalizeStoredTimeAgreesWithFormatStoredTime is what makes the stale
// sweep correct: whatever protojson wrote for scanned_at must normalize to the
// exact same string the sweep's cutoff is formatted as, or the "$lt" string
// comparison is meaningless.
func TestNormalizeStoredTimeAgreesWithFormatStoredTime(t *testing.T) {
	instants := []time.Time{
		time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
		time.Date(2026, 1, 2, 15, 4, 5, 500_000_000, time.UTC),
		time.Date(2026, 1, 2, 15, 4, 5, 123_456_789, time.UTC),
		time.Now().UTC(),
	}

	for _, instant := range instants {
		encoded, err := MarshalStored(&TVSeries{ScannedAt: timestamppb.New(instant)})
		if err != nil {
			t.Fatalf("MarshalStored() error = %v", err)
		}
		var fields struct {
			ScannedAt string `json:"scanned_at"`
		}
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatalf("decoding MarshalStored output: %v", err)
		}

		normalized := NormalizeStoredTime(fields.ScannedAt)
		if want := FormatStoredTime(instant); normalized != want {
			t.Errorf("NormalizeStoredTime(%q) = %q, want %q", fields.ScannedAt, normalized, want)
		}
		if len(normalized) != len("2026-01-02T15:04:05.000000000Z") {
			t.Errorf("normalized form %q is not fixed-width", normalized)
		}
	}
}

// TestFormatStoredTimeSortsChronologically covers the property "$lt" on the
// scanned_at string relies on: lexicographic order of the formatted values is
// the same as time order, including across sub-second precision boundaries.
func TestFormatStoredTimeSortsChronologically(t *testing.T) {
	base := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	ordered := []time.Time{
		base,
		base.Add(1 * time.Nanosecond),
		base.Add(500 * time.Microsecond),
		base.Add(123 * time.Millisecond),
		base.Add(900 * time.Millisecond),
		base.Add(1 * time.Second),
		base.Add(61 * time.Second),
		base.Add(24 * time.Hour),
	}

	for i := 1; i < len(ordered); i++ {
		earlier := FormatStoredTime(ordered[i-1])
		later := FormatStoredTime(ordered[i])
		if earlier >= later {
			t.Errorf("formatted %q (for %v) is not < %q (for %v)", earlier, ordered[i-1], later, ordered[i])
		}
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
