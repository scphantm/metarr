package agentproto

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestPresenceKeyRoundTripsThroughSlugFromPresenceKey(t *testing.T) {
	for _, slug := range []string{"nas-01", "a", "local_agent", "agent-with-a-long-name-99"} {
		if got := SlugFromPresenceKey(PresenceKey(slug)); got != slug {
			t.Errorf("SlugFromPresenceKey(PresenceKey(%q)) = %q", slug, got)
		}
	}
}

// The scan pattern has to match the keys the builder produces, or the server
// discovers nothing and the dashboard silently shows no agent streams.
func TestKeyBuildersAgreeWithTheirPatterns(t *testing.T) {
	if want := "metarr:agent:presence:nas-01"; PresenceKey("nas-01") != want {
		t.Errorf("PresenceKey = %q, want %q", PresenceKey("nas-01"), want)
	}
	if got := SlugFromPresenceKey("metarr:agent:presence:"); got != "" {
		t.Errorf("a key with no slug returned %q, want empty", got)
	}
}

func TestValidateSlugAcceptsUsableNames(t *testing.T) {
	for _, slug := range []string{"nas-01", "local", "a1_b2-c3"} {
		if err := ValidateSlug(slug); err != nil {
			t.Errorf("ValidateSlug(%q) = %v, want nil", slug, err)
		}
	}
}

// A slug is embedded directly into Redis keys and stream names, so anything
// that would break those has to be refused at the edge.
func TestValidateSlugRejectsUnsafeNames(t *testing.T) {
	cases := map[string]string{
		"empty":     "",
		"uppercase": "NAS-01",
		"colon":     "nas:01",
		"dot":       "nas.01",
		"space":     "nas 01",
		"glob":      "nas*",
		"toolong":   "a123456789012345678901234567890123456789012345678901234567890123",
	}
	for name, slug := range cases {
		if err := ValidateSlug(slug); err == nil {
			t.Errorf("%s: ValidateSlug(%q) = nil, want an error", name, slug)
		}
	}
}

func TestFindDirectoryReturnsOnlyMappedLibraries(t *testing.T) {
	projection := &AgentConfigProjection{
		Directories: []*MappedDirectory{
			{ScannerSlug: "movies", ScanType: "movie", AgentPath: "/mnt/tank/movies"},
		},
	}

	directory, ok := FindDirectory(projection, "movies")
	if !ok || directory.AgentPath != "/mnt/tank/movies" {
		t.Errorf("FindDirectory(movies) = %+v, %v", directory, ok)
	}
	if _, ok := FindDirectory(projection, "tv"); ok {
		t.Error("FindDirectory returned an unmapped library")
	}
}

// TestMarshalStoredRoundTrip pins the Redis serialization contract: proto
// field names (snake_case) and an RFC 3339 timestamp, decodable back to an
// equal message.
func TestMarshalStoredRoundTrip(t *testing.T) {
	original := &AgentPresence{
		Identity:   &AgentIdentity{Slug: "nas", InstanceId: "run-1", Uid: 1000},
		Telemetry:  &AgentTelemetry{CpuPercent: 12.5, MemoryUsedBytes: 4 << 30},
		ReportedAt: timestamppb.New(time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)),
	}

	encoded, err := MarshalStored(original)
	if err != nil {
		t.Fatalf("MarshalStored() error = %v", err)
	}
	for _, want := range []string{`"instance_id"`, `"cpu_percent"`, `"2026-01-02T15:04:05Z"`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("encoded form %s is missing %s", encoded, want)
		}
	}

	var readBack AgentPresence
	if err := UnmarshalStored(encoded, &readBack); err != nil {
		t.Fatalf("UnmarshalStored() error = %v", err)
	}
	if !proto.Equal(original, &readBack) {
		t.Errorf("round trip changed the record:\n got %v\nwant %v", &readBack, original)
	}
}
