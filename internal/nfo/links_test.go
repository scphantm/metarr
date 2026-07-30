package nfo

import (
	"testing"
)

// linksEqual compares ignoring order, since callers only query these.
func linksContain(links []Link, want Link) bool {
	for _, link := range links {
		if link == want {
			return true
		}
	}
	return false
}

func TestExtractLinksFromUniqueIDs(t *testing.T) {
	doc := &Document{Kind: KindMovie, Movie: &Movie{
		UniqueIDs: []UniqueID{
			{Type: "tmdb", Default: true, Value: "603"},
			{Type: "imdb", Value: "tt0133093"},
		},
	}}

	links := ExtractLinks(doc)
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
	doc := &Document{Kind: KindTVShow, TVShow: &TVShow{
		UniqueIDs: []UniqueID{
			{Type: "tvdbid", Value: "81189"},
			{Type: "TheMovieDB", Value: "1396"},
			{Type: "imdbid", Value: "tt0903747"},
		},
	}}

	links := ExtractLinks(doc)
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
	doc := &Document{Kind: KindMovie, Movie: &Movie{
		UniqueIDs: []UniqueID{{Type: "SomeNewDB", Value: "xyz-42"}},
	}}

	links := ExtractLinks(doc)
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
			doc := &Document{Kind: KindMovie, Movie: &Movie{ID: test.id}}
			links := ExtractLinks(doc)
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
		{
			name:    "kodi plugin url",
			trailer: "plugin://plugin.video.youtube/?action=play_video&videoid=dQw4w9WgXcQ",
			want:    "dQw4w9WgXcQ",
		},
		{
			name:    "watch url",
			trailer: "https://www.youtube.com/watch?v=vKQi3bBA1y8",
			want:    "vKQi3bBA1y8",
		},
		{
			name:    "short url",
			trailer: "https://youtu.be/m8e-FF8MsqU",
			want:    "m8e-FF8MsqU",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := &Document{Kind: KindMovie, Movie: &Movie{Trailer: test.trailer}}
			links := ExtractLinks(doc)
			if !linksContain(links, Link{Key: "youtube", Value: test.want}) {
				t.Errorf("links = %+v, want youtube=%s", links, test.want)
			}
		})
	}
}

func TestExtractLinksIgnoresNonYouTubeTrailer(t *testing.T) {
	doc := &Document{Kind: KindMovie, Movie: &Movie{Trailer: "/media/trailers/local-trailer.mkv"}}
	if links := ExtractLinks(doc); len(links) != 0 {
		t.Errorf("links = %+v, want none for a local trailer path", links)
	}
}

func TestExtractLinksEpisodeGuide(t *testing.T) {
	tests := []struct {
		name  string
		guide string
		want  Link
	}{
		{"string value", `{"tvdb":"81189"}`, Link{Key: "tvdb", Value: "81189"}},
		{"numeric value", `{"tvdb":81189}`, Link{Key: "tvdb", Value: "81189"}},
		{"aliased key", `{"tvdbid":"81189"}`, Link{Key: "tvdb", Value: "81189"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := &Document{Kind: KindTVShow, TVShow: &TVShow{EpisodeGuide: test.guide}}
			links := ExtractLinks(doc)
			if !linksContain(links, test.want) {
				t.Errorf("links = %+v, want %+v", links, test.want)
			}
		})
	}
}

func TestExtractLinksIgnoresLegacyEpisodeGuideURL(t *testing.T) {
	// Pre-v19 Kodi wrote a nested <url> element here, which carries no id.
	doc := &Document{Kind: KindTVShow, TVShow: &TVShow{
		EpisodeGuide: `<url post="yes" cache="x.xml">http://example.com/api</url>`,
	}}
	if links := ExtractLinks(doc); len(links) != 0 {
		t.Errorf("links = %+v, want none", links)
	}
}

// TestExtractLinksDeduplicates matters because the same id routinely appears in
// both <uniqueid> and the legacy <id> tag.
func TestExtractLinksDeduplicates(t *testing.T) {
	doc := &Document{Kind: KindMovie, Movie: &Movie{
		UniqueIDs: []UniqueID{
			{Type: "imdb", Value: "tt0133093"},
			{Type: "imdb", Value: "tt0133093"},
		},
		ID: "tt0133093",
	}}

	links := ExtractLinks(doc)
	if len(links) != 1 {
		t.Fatalf("links = %+v, want exactly 1 after dedupe", links)
	}
}

func TestExtractLinksFromEpisodes(t *testing.T) {
	doc := &Document{Kind: KindEpisode, Episodes: []EpisodeDetails{
		{UniqueIDs: []UniqueID{{Type: "tvdb", Value: "349232"}}},
		{UniqueIDs: []UniqueID{{Type: "tvdb", Value: "349233"}}},
	}}

	links := ExtractLinks(doc)
	if len(links) != 2 {
		t.Fatalf("links = %+v, want 2", links)
	}
}

func TestExtractLinksSkipsEmptyValues(t *testing.T) {
	doc := &Document{Kind: KindMovie, Movie: &Movie{
		UniqueIDs: []UniqueID{
			{Type: "tmdb", Value: ""},
			{Type: "", Value: ""},
			{Type: "imdb", Value: "   "},
		},
		ID:      "",
		Trailer: "",
	}}

	if links := ExtractLinks(doc); len(links) != 0 {
		t.Errorf("links = %+v, want none", links)
	}
}

func TestExtractLinksTypelessUniqueID(t *testing.T) {
	// A uniqueid without a type attribute still carries an id; it is attributed
	// the same way as the legacy <id> tag rather than discarded.
	doc := &Document{Kind: KindMovie, Movie: &Movie{
		UniqueIDs: []UniqueID{{Value: "tt0133093"}},
	}}

	links := ExtractLinks(doc)
	if !linksContain(links, Link{Key: "imdb", Value: "tt0133093"}) {
		t.Errorf("links = %+v", links)
	}
}

func TestExtractLinksNilSafe(t *testing.T) {
	if links := ExtractLinks(nil); links != nil {
		t.Errorf("ExtractLinks(nil) = %+v, want nil", links)
	}
	if links := ExtractLinks(&Document{Kind: KindUnknown}); links != nil {
		t.Errorf("links = %+v, want nil", links)
	}
}

// TestExtractLinksEndToEnd runs the extractor over a parsed file rather than a
// hand-built struct, so the tag plumbing is covered too.
func TestExtractLinksEndToEnd(t *testing.T) {
	doc, err := Parse([]byte(`<movie>
    <title>The Matrix</title>
    <uniqueid type="tmdb" default="true">603</uniqueid>
    <uniqueid type="imdb">tt0133093</uniqueid>
    <id>tt0133093</id>
    <trailer>plugin://plugin.video.youtube/?action=play_video&amp;videoid=vKQi3bBA1y8</trailer>
</movie>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	links := ExtractLinks(doc)
	for _, want := range []Link{
		{Key: "tmdb", Value: "603"},
		{Key: "imdb", Value: "tt0133093"},
		{Key: "youtube", Value: "vKQi3bBA1y8"},
	} {
		if !linksContain(links, want) {
			t.Errorf("missing %+v in %+v", want, links)
		}
	}
	if len(links) != 3 {
		t.Errorf("links = %+v, want exactly 3 (the duplicate <id> folded away)", links)
	}
}
