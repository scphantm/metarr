package nfo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestMarshalIncludesDeclaration(t *testing.T) {
	doc := &Document{Kind: KindMovie, Movie: &Movie{Title: "The Matrix"}}

	data, err := Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	out := string(data)
	if !strings.HasPrefix(out, `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>`) {
		t.Errorf("output missing Kodi's XML declaration:\n%s", out)
	}
	if !strings.Contains(out, "<title>The Matrix</title>") {
		t.Errorf("output missing title:\n%s", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("output should end with a newline")
	}
}

// TestRoundTripPreservesUnknownTags is the most important test in this package.
// NFO files are Metarr's system of record and other tools write tags Metarr
// doesn't model; if a rewrite dropped them it would destroy user metadata.
func TestRoundTripPreservesUnknownTags(t *testing.T) {
	original := `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>
<movie>
    <title>The Matrix</title>
    <runtime>136</runtime>
    <somethingmetarrdoesnotknow>preserve me</somethingmetarrdoesnotknow>
    <customblock attr="kept">
        <nested>deep value</nested>
    </customblock>
</movie>`

	doc, err := Parse([]byte(original))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(doc.Movie.Extra) != 2 {
		t.Fatalf("len(Extra) = %d, want 2; unknown tags were not captured: %+v", len(doc.Movie.Extra), doc.Movie.Extra)
	}

	data, err := Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	out := string(data)

	for _, want := range []string{
		"<somethingmetarrdoesnotknow>preserve me</somethingmetarrdoesnotknow>",
		"<nested>deep value</nested>",
		`attr="kept"`,
		"<title>The Matrix</title>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("round-tripped output lost %q:\n%s", want, out)
		}
	}

	// And it must survive a second trip, so repeated saves don't erode the file.
	reparsed, err := Parse(data)
	if err != nil {
		t.Fatalf("re-Parse() error = %v", err)
	}
	if len(reparsed.Movie.Extra) != 2 {
		t.Errorf("second round trip has len(Extra) = %d, want 2", len(reparsed.Movie.Extra))
	}
}

// TestMarshalAfterStorageRoundTrip covers the case where a document came back
// from a store that doesn't persist xml.Name: the element name has to be
// recovered from the plain Name field, or preserved tags would be emitted
// unnamed.
func TestMarshalAfterStorageRoundTrip(t *testing.T) {
	doc, err := Parse([]byte(`<movie><title>x</title><weirdtag>value</weirdtag></movie>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.Movie.Extra[0].Name != "weirdtag" {
		t.Fatalf("Extra[0].Name = %q, want %q", doc.Movie.Extra[0].Name, "weirdtag")
	}

	// Simulate the loss of xml.Name that BSON storage causes.
	doc.Movie.Extra[0].XMLName.Local = ""

	data, err := Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), "<weirdtag>value</weirdtag>") {
		t.Errorf("preserved tag lost after storage round trip:\n%s", data)
	}
}

func TestMarshalEachRootType(t *testing.T) {
	tests := []struct {
		name    string
		doc     *Document
		wantTag string
	}{
		{"movie", &Document{Kind: KindMovie, Movie: &Movie{Title: "m"}}, "<movie>"},
		{"tvshow", &Document{Kind: KindTVShow, TVShow: &TVShow{Title: "s"}}, "<tvshow>"},
		{"musicvideo", &Document{Kind: KindMusicVideo, MusicVideo: &MusicVideo{Title: "v"}}, "<musicvideo>"},
		{"episode", &Document{Kind: KindEpisode, Episodes: []EpisodeDetails{{Title: "e"}}}, "<episodedetails>"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := Marshal(test.doc)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if !strings.Contains(string(data), test.wantTag) {
				t.Errorf("output missing %q:\n%s", test.wantTag, data)
			}
		})
	}
}

// TestMarshalRejectsMultipleEpisodes covers the read-both/write-single rule:
// Kodi v22 dropped multi-root episode files, so emitting one would produce a
// file newer Kodi ignores.
func TestMarshalRejectsMultipleEpisodes(t *testing.T) {
	doc := &Document{
		Kind:     KindEpisode,
		Episodes: []EpisodeDetails{{Title: "one"}, {Title: "two"}},
	}

	_, err := Marshal(doc)
	if !errors.Is(err, ErrMultipleEpisodes) {
		t.Fatalf("Marshal() error = %v, want ErrMultipleEpisodes", err)
	}
}

func TestMarshalRejectsEmptyDocuments(t *testing.T) {
	if _, err := Marshal(nil); err == nil {
		t.Error("Marshal(nil) succeeded")
	}
	if _, err := Marshal(&Document{Kind: KindURL}); err == nil {
		t.Error("Marshal() succeeded on a document with no content")
	}
}

func TestWriteFileCreatesAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.nfo")

	doc := &Document{Kind: KindMovie, Movie: &Movie{Title: "Fresh"}}
	if err := WriteFile(path, doc); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if !strings.Contains(string(written), "<title>Fresh</title>") {
		t.Errorf("content = %s", written)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Errorf("directory holds %v, want only movie.nfo; a temp file was left behind", names)
	}
}

func TestWriteFilePreservesExistingMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.nfo")
	if err := writeTestFile(t, path, `<movie><title>old</title></movie>`); err != nil {
		t.Fatal(err)
	}
	const mode = os.FileMode(0o640)
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}

	doc := &Document{Kind: KindMovie, Movie: &Movie{Title: "new"}}
	if err := WriteFile(path, doc); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Errorf("mode = %v, want %v; a rewrite must not change library permissions", info.Mode().Perm(), mode)
	}
}

func TestWriteFileReplacesContentEntirely(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.nfo")
	if err := writeTestFile(t, path, `<movie><title>a much longer previous title</title></movie>`); err != nil {
		t.Fatal(err)
	}

	if err := WriteFile(path, &Document{Kind: KindMovie, Movie: &Movie{Title: "short"}}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "previous title") {
		t.Errorf("stale content survived the replace:\n%s", written)
	}
}

func TestWriteFileErrorsOnBadDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.nfo")

	doc := &Document{Kind: KindEpisode, Episodes: []EpisodeDetails{{Title: "a"}, {Title: "b"}}}
	if err := WriteFile(path, doc); !errors.Is(err, ErrMultipleEpisodes) {
		t.Fatalf("WriteFile() error = %v, want ErrMultipleEpisodes", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("WriteFile() created a file despite failing to marshal")
	}
}
