package agentproto

import "testing"

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
	if want := "events.agent.nas-01.commands"; CommandStream("nas-01") != want {
		t.Errorf("CommandStream = %q, want %q", CommandStream("nas-01"), want)
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
	projection := AgentConfigProjection{
		Directories: []MappedDirectory{
			{ScannerSlug: "movies", ScanType: "movie", AgentPath: "/mnt/tank/movies"},
		},
	}

	directory, ok := projection.FindDirectory("movies")
	if !ok || directory.AgentPath != "/mnt/tank/movies" {
		t.Errorf("FindDirectory(movies) = %+v, %v", directory, ok)
	}
	if _, ok := projection.FindDirectory("tv"); ok {
		t.Error("FindDirectory returned an unmapped library")
	}
}
