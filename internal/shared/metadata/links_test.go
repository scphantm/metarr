package metadata

import "testing"

// linksContain compares ignoring order, since callers only query these, and
// ignoring the default flag, which is an attribute of a link rather than part of
// its identity.
func linksContain(links []Link, want Link) bool {
	for _, link := range links {
		if link.Key == want.Key && link.Value == want.Value {
			return true
		}
	}
	return false
}

func TestExtractLinksFromUniqueIDs(t *testing.T) {
	links := LinksFromUniqueIDs([]UniqueID{
		{Type: "tmdb", Default: true, Value: "603"},
		{Type: "imdb", Value: "tt0133093"},
	})
	if len(links) != 2 {
		t.Fatalf("links = %+v, want 2", links)
	}
	for _, want := range []Link{{Key: "tmdb", Value: "603"}, {Key: "imdb", Value: "tt0133093"}} {
		if !linksContain(links, want) {
			t.Errorf("missing %+v in %+v", want, links)
		}
	}
}

// TestExtractLinksFoldsAliases covers the same provider appearing under
// different spellings, so a lookup doesn't have to try every variant.
func TestExtractLinksFoldsAliases(t *testing.T) {
	links := LinksFromUniqueIDs([]UniqueID{
		{Type: "tvdbid", Value: "81189"},
		{Type: "TheMovieDB", Value: "1396"},
		{Type: "imdbid", Value: "tt0903747"},
	})
	for _, want := range []Link{
		{Key: "tvdb", Value: "81189"},
		{Key: "tmdb", Value: "1396"},
		{Key: "imdb", Value: "tt0903747"},
	} {
		if !linksContain(links, want) {
			t.Errorf("missing %+v in %+v", want, links)
		}
	}
}

// TestExtractLinksKeepsUnknownProviders is the behaviour that makes "any other
// external system" work: a provider Metarr has never heard of is still
// captured rather than dropped.
func TestExtractLinksKeepsUnknownProviders(t *testing.T) {
	links := LinksFromUniqueIDs([]UniqueID{{Type: "SomeNewDB", Value: "xyz-42"}})
	if !linksContain(links, Link{Key: "somenewdb", Value: "xyz-42"}) {
		t.Errorf("unknown provider was dropped: %+v", links)
	}
}

func TestExtractLinksLegacyIDTag(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want Link
	}{
		{"imdb shaped", "tt0133093", Link{Key: "imdb", Value: "tt0133093"}},
		{"numeric falls back", "81189", Link{Key: "id", Value: "81189"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			links := ExtractLinks(&Metadata{ID: test.id})
			if !linksContain(links, test.want) {
				t.Errorf("links = %+v, want %+v", links, test.want)
			}
		})
	}
}

func TestExtractLinksYouTubeTrailers(t *testing.T) {
	tests := []struct {
		name    string
		trailer string
		want    string
	}{
		{"kodi plugin url", "plugin://plugin.video.youtube/?action=play_video&videoid=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"watch url", "https://www.youtube.com/watch?v=vKQi3bBA1y8", "vKQi3bBA1y8"},
		{"short url", "https://youtu.be/m8e-FF8MsqU", "m8e-FF8MsqU"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			links := ExtractLinks(&Metadata{Trailer: test.trailer})
			if !linksContain(links, Link{Key: "youtube", Value: test.want}) {
				t.Errorf("links = %+v, want youtube=%s", links, test.want)
			}
		})
	}
}

func TestExtractLinksIgnoresNonYouTubeTrailer(t *testing.T) {
	if links := ExtractLinks(&Metadata{Trailer: "/media/trailers/local-trailer.mkv"}); len(links) != 0 {
		t.Errorf("links = %+v, want none for a local trailer path", links)
	}
}

// TestExtractLinksDeduplicates matters because the same id routinely appears in
// both a uniqueid and the legacy id.
func TestExtractLinksDeduplicates(t *testing.T) {
	links := ExtractLinks(&Metadata{
		ExternalLinks: LinksFromUniqueIDs([]UniqueID{
			{Type: "imdb", Value: "tt0133093"},
			{Type: "imdb", Value: "tt0133093"},
		}),
		ID: "tt0133093",
	})
	if len(links) != 1 {
		t.Fatalf("links = %+v, want exactly 1 after dedupe", links)
	}
}

func TestExtractLinksSkipsEmptyValues(t *testing.T) {
	links := ExtractLinks(&Metadata{
		ExternalLinks: LinksFromUniqueIDs([]UniqueID{
			{Type: "tmdb", Value: ""},
			{Type: "", Value: ""},
			{Type: "imdb", Value: "   "},
		}),
		ID:      "",
		Trailer: "",
	})
	if len(links) != 0 {
		t.Errorf("links = %+v, want none", links)
	}
}

func TestExtractLinksTypelessUniqueID(t *testing.T) {
	// A uniqueid without a type attribute still carries an id; it is attributed
	// the same way as the legacy id rather than discarded.
	links := LinksFromUniqueIDs([]UniqueID{{Value: "tt0133093"}})
	if !linksContain(links, Link{Key: "imdb", Value: "tt0133093"}) {
		t.Errorf("links = %+v", links)
	}
}

func TestExtractLinksNilSafe(t *testing.T) {
	if links := ExtractLinks(nil); links != nil {
		t.Errorf("ExtractLinks(nil) = %+v, want nil", links)
	}
	if links := ExtractLinks(&Metadata{Kind: KindUnknown}); links != nil {
		t.Errorf("links = %+v, want nil", links)
	}
}

// TestLinksCarryTheDefaultFlag covers the attribute that has no reader in Metarr
// but has to survive anyway, because the NFO file is the system of record.
func TestLinksCarryTheDefaultFlag(t *testing.T) {
	links := LinksFromUniqueIDs([]UniqueID{
		{Type: "tmdb", Value: "603", Default: true},
		{Type: "imdb", Value: "tt0133093"},
	})

	for _, link := range links {
		switch link.Key {
		case "tmdb":
			if !link.Default {
				t.Error("the default flag was dropped on read")
			}
		case "imdb":
			if link.Default {
				t.Error("a link that was not the default came back flagged")
			}
		}
	}
}

// TestDefaultFlagSurvivesDuplicates covers the same id arriving twice, once
// flagged and once not: that is one link, and it keeps the flag.
func TestDefaultFlagSurvivesDuplicates(t *testing.T) {
	links := LinksFromUniqueIDs([]UniqueID{
		{Type: "tmdb", Value: "603"},
		{Type: "tmdb", Value: "603", Default: true},
	})
	if len(links) != 1 {
		t.Fatalf("links = %+v, want exactly 1 after dedupe", links)
	}
	if !links[0].Default {
		t.Error("the default flag was lost when the duplicate was folded away")
	}
}

func TestUniqueIDsFromLinksRoundTrip(t *testing.T) {
	original := []UniqueID{
		{Type: "tmdb", Value: "603", Default: true},
		{Type: "imdb", Value: "tt0133093"},
	}

	rebuilt := UniqueIDsFromLinks(LinksFromUniqueIDs(original))
	if len(rebuilt) != len(original) {
		t.Fatalf("rebuilt = %+v, want %d entries", rebuilt, len(original))
	}
	for i, want := range original {
		if rebuilt[i] != want {
			t.Errorf("rebuilt[%d] = %+v, want %+v", i, rebuilt[i], want)
		}
	}
}

// TestUniqueIDsFromLinksSkipsDerivedKeys is what stops a rewrite inventing tags:
// the ids ExtractLinks synthesizes from <id> and <trailer> are re-emitted from
// those fields, so turning them into uniqueid elements would add content the
// source file never had.
func TestUniqueIDsFromLinksSkipsDerivedKeys(t *testing.T) {
	links := ExtractLinks(&Metadata{
		ExternalLinks: LinksFromUniqueIDs([]UniqueID{{Type: "tmdb", Value: "603"}}),
		ID:            "12345",
		Trailer:       "plugin://plugin.video.youtube/?action=play_video&videoid=HhesaQXLuRY",
	})
	if !linksContain(links, Link{Key: "id", Value: "12345"}) || !linksContain(links, Link{Key: "youtube", Value: "HhesaQXLuRY"}) {
		t.Fatalf("ExtractLinks should surface the derived ids for querying: %+v", links)
	}

	rebuilt := UniqueIDsFromLinks(links)
	if len(rebuilt) != 1 || rebuilt[0].Type != "tmdb" {
		t.Errorf("rebuilt = %+v, want only the tmdb tag the file actually carried", rebuilt)
	}
}

func TestUniqueIDsFromLinksEmpty(t *testing.T) {
	if got := UniqueIDsFromLinks(nil); got != nil {
		t.Errorf("UniqueIDsFromLinks(nil) = %+v, want nil so the element is omitted", got)
	}
	if got := UniqueIDsFromLinks([]Link{{Key: "id", Value: "12345"}}); got != nil {
		t.Errorf("a table of only derived keys should yield nil, got %+v", got)
	}
}
