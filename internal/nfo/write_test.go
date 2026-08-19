package nfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Metarr/internal/metadata"
)

func writeTestFile(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0o644)
}

// render is the write half in isolation: metadata → on-disk bytes.
func render(t *testing.T, md *metadata.Metadata) string {
	t.Helper()
	data, err := marshal(documentFromMetadata(md))
	if err != nil {
		t.Fatalf("marshal() error = %v", err)
	}
	return string(data)
}

func TestMarshalIncludesDeclaration(t *testing.T) {
	out := render(t, &metadata.Metadata{Kind: metadata.KindMovie, Title: "The Matrix"})
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
// NFO files are a system of record and other tools write tags Metarr doesn't
// model; a rewrite that dropped them would destroy user metadata. They ride
// through the metadata model in Extra.
func TestRoundTripPreservesUnknownTags(t *testing.T) {
	md := mustParse(t, `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>
<movie>
    <title>The Matrix</title>
    <runtime>136</runtime>
    <somethingmetarrdoesnotknow>preserve me</somethingmetarrdoesnotknow>
    <customblock attr="kept">
        <nested>deep value</nested>
    </customblock>
</movie>`).toMetadata()

	if len(md.Extra) != 2 {
		t.Fatalf("len(Extra) = %d, want 2; unknown tags were not captured: %+v", len(md.Extra), md.Extra)
	}

	out := render(t, md)
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
	reparsed := mustParse(t, out).toMetadata()
	if len(reparsed.Extra) != 2 {
		t.Errorf("second round trip has len(Extra) = %d, want 2", len(reparsed.Extra))
	}
}

// TestMarshalAfterStorageRoundTrip covers a record that came back from a store
// that doesn't persist xml.Name: the element name has to be recovered from the
// plain Name field, or preserved tags would be emitted unnamed.
func TestMarshalAfterStorageRoundTrip(t *testing.T) {
	md := mustParse(t, `<movie><title>x</title><weirdtag>value</weirdtag></movie>`).toMetadata()
	if md.Extra[0].Name != "weirdtag" {
		t.Fatalf("Extra[0].Name = %q, want %q", md.Extra[0].Name, "weirdtag")
	}

	// Simulate the loss of xml.Name that BSON storage causes.
	md.Extra[0].XMLName.Local = ""

	if out := render(t, md); !strings.Contains(out, "<weirdtag>value</weirdtag>") {
		t.Errorf("preserved tag lost after storage round trip:\n%s", out)
	}
}

func TestMarshalEachRootType(t *testing.T) {
	tests := []struct {
		name    string
		md      *metadata.Metadata
		wantTag string
	}{
		{"movie", &metadata.Metadata{Kind: metadata.KindMovie, Title: "m"}, "<movie>"},
		{"tvshow", &metadata.Metadata{Kind: metadata.KindTVShow, Title: "s"}, "<tvshow>"},
		{"musicvideo", &metadata.Metadata{Kind: metadata.KindMusicVideo, Title: "v"}, "<musicvideo>"},
		{"episode", &metadata.Metadata{Kind: metadata.KindEpisode, Title: "e"}, "<episodedetails>"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if out := render(t, test.md); !strings.Contains(out, test.wantTag) {
				t.Errorf("output missing %q:\n%s", test.wantTag, out)
			}
		})
	}
}

func TestMarshalRejectsEmptyDocuments(t *testing.T) {
	if _, err := marshal(nil); err == nil {
		t.Error("marshal(nil) succeeded")
	}
	if _, err := marshal(documentFromMetadata(&metadata.Metadata{Kind: metadata.KindURL})); err == nil {
		t.Error("marshal() succeeded on a document with no content")
	}
}

func TestWriteFileCreatesAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.nfo")

	if err := WriteFile(path, &metadata.Metadata{Kind: metadata.KindMovie, Title: "Fresh"}); err != nil {
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

	if err := WriteFile(path, &metadata.Metadata{Kind: metadata.KindMovie, Title: "new"}); err != nil {
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

	if err := WriteFile(path, &metadata.Metadata{Kind: metadata.KindMovie, Title: "short"}); err != nil {
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

func TestWriteFileErrorsOnEmptyDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.nfo")

	if err := WriteFile(path, &metadata.Metadata{Kind: metadata.KindUnknown}); err == nil {
		t.Fatal("WriteFile() succeeded on a record with no writable content")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("WriteFile() created a file despite failing to marshal")
	}
}

// TestRoundTripPreservesFileInfo covers a tag that used to be modelled and no
// longer is. Dropping the typed field must not mean dropping the data: NFO
// files are the system of record, so <fileinfo> has to come back out of a save
// exactly as it went in, carried by the unknown-element mechanism.
func TestRoundTripPreservesFileInfo(t *testing.T) {
	md := mustParse(t, `<movie>
    <title>The Matrix</title>
    <fileinfo>
        <streamdetails>
            <video>
                <codec>h264</codec>
                <height>1080</height>
            </video>
        </streamdetails>
    </fileinfo>
</movie>`).toMetadata()

	out := render(t, md)
	for _, want := range []string{"<fileinfo>", "<codec>h264</codec>", "<height>1080</height>"} {
		if !strings.Contains(out, want) {
			t.Errorf("round-tripped output lost %q:\n%s", want, out)
		}
	}

	// A second save must not erode it either.
	if reparsed := render(t, mustParse(t, out).toMetadata()); !strings.Contains(reparsed, "<codec>h264</codec>") {
		t.Errorf("stream details lost on the second round trip:\n%s", reparsed)
	}
}

// TestRoundTripDates pins what modelling the date tags as dates costs. A bare
// date survives untouched; a tag written with a time of day comes back as the
// day alone, because the model records a date rather than an instant.
func TestRoundTripDates(t *testing.T) {
	md := mustParse(t, `<movie>
    <title>The Matrix</title>
    <premiered>1999-03-31</premiered>
    <dateadded>2008-01-20 10:35:00</dateadded>
</movie>`).toMetadata()

	if got := metadata.FormatDate(md.Premiered); got != "1999-03-31" {
		t.Errorf("Premiered = %q, want %q", got, "1999-03-31")
	}
	if got := metadata.FormatDate(md.DateAdded); got != "2008-01-20" {
		t.Errorf("DateAdded = %q, want %q — the time of day is not modelled", got, "2008-01-20")
	}

	out := render(t, md)
	if !strings.Contains(out, "<premiered>1999-03-31</premiered>") {
		t.Errorf("premiered did not survive the round trip:\n%s", out)
	}
	if !strings.Contains(out, "<dateadded>2008-01-20</dateadded>") {
		t.Errorf("dateadded did not survive the round trip:\n%s", out)
	}
}

// TestUnparseableDateIsDroppedNotFatal covers a real-world badly written tag:
// it must not fail the read, and it must not be written back as junk.
func TestUnparseableDateIsDroppedNotFatal(t *testing.T) {
	md := mustParse(t, `<movie><title>x</title><premiered>not a date</premiered></movie>`).toMetadata()

	if !md.Premiered.Time.IsZero() {
		t.Errorf("Premiered = %v, want the zero date", md.Premiered)
	}
	if out := render(t, md); strings.Contains(out, "<premiered>") {
		t.Errorf("an unparseable date was written back out:\n%s", out)
	}
}

// TestRoundTripPreservesUniqueIDs covers the provider ids now that they live in
// ExternalLinks rather than a field of their own: they have to come back out of
// a save as the same <uniqueid> tags, default attribute included.
func TestRoundTripPreservesUniqueIDs(t *testing.T) {
	md := mustParse(t, `<movie>
    <title>The Matrix</title>
    <uniqueid type="tmdb" default="true">603</uniqueid>
    <uniqueid type="imdb">tt0133093</uniqueid>
</movie>`).toMetadata()

	out := render(t, md)
	for _, want := range []string{
		`<uniqueid type="tmdb" default="true">603</uniqueid>`,
		`<uniqueid type="imdb">tt0133093</uniqueid>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("round-tripped output lost %q:\n%s", want, out)
		}
	}
}

// TestRoundTripDoesNotInventUniqueIDs is the guard on the one place the new
// shape could corrupt a file. ExtractLinks folds ids out of <id> and <trailer>
// into the same ExternalLinks list the uniqueid tags use, and the scanner
// assigns that union back onto the record — so the writer has to know which of
// those were never uniqueid tags to begin with.
func TestRoundTripDoesNotInventUniqueIDs(t *testing.T) {
	md := mustParse(t, `<movie>
    <title>The Matrix</title>
    <uniqueid type="tmdb">603</uniqueid>
    <id>12345</id>
    <trailer>plugin://plugin.video.youtube/?action=play_video&amp;videoid=HhesaQXLuRY</trailer>
</movie>`).toMetadata()

	// Stand in for what the scanner does: replace the links with the derived union.
	md.ExternalLinks = metadata.ExtractLinks(md)

	out := render(t, md)
	if strings.Count(out, "<uniqueid") != 1 {
		t.Errorf("expected exactly the one uniqueid the file carried:\n%s", out)
	}
	for _, unwanted := range []string{`type="youtube"`, `type="id"`} {
		if strings.Contains(out, unwanted) {
			t.Errorf("a derived id was written back as a uniqueid (%s):\n%s", unwanted, out)
		}
	}
	// The tags those ids really came from are still there.
	if !strings.Contains(out, "<id>12345</id>") || !strings.Contains(out, "videoid=HhesaQXLuRY") {
		t.Errorf("the source tags were lost:\n%s", out)
	}
}
