package eventbus

import (
	"testing"
	"time"
)

func TestStreamIDRoundTrip(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	id := StreamIDForTime(at)
	if id != "1772366400000-0" {
		t.Fatalf("StreamIDForTime = %q, want 1772366400000-0", id)
	}

	when, ok := TimeFromStreamID(id)
	if !ok {
		t.Fatal("TimeFromStreamID rejected an ID it just produced")
	}
	if !when.Equal(at) {
		t.Errorf("round trip = %v, want %v", when, at)
	}
}

func TestTimeFromStreamIDIgnoresTheSequenceAndRejectsGarbage(t *testing.T) {
	when, ok := TimeFromStreamID("1700000000000-42")
	if !ok || when.UnixMilli() != 1700000000000 {
		t.Errorf("TimeFromStreamID(...-42) = %v, %v", when, ok)
	}
	if _, ok := TimeFromStreamID("not-an-id"); ok {
		t.Error("expected a malformed ID to be rejected")
	}
}
