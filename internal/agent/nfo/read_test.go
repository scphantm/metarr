package nfo

import (
	"strings"
	"testing"

	"Metarr/internal/shared/metadata"
)

func mustParse(t *testing.T, data string) *document {
	t.Helper()
	doc, err := parse([]byte(data))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	return doc
}

func TestParseMovie(t *testing.T) {
	md := mustParse(t, `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>
<movie>
    <title>The Matrix</title>
    <originaltitle>The Matrix</originaltitle>
    <sorttitle>Matrix, The</sorttitle>
    <ratings>
        <rating name="themoviedb" max="10" default="true">
            <value>8.2</value>
            <votes>24601</votes>
        </rating>
    </ratings>
    <userrating>9</userrating>
    <plot>A hacker learns the truth.</plot>
    <runtime>136</runtime>
    <thumb aspect="poster" preview="small.jpg">poster.jpg</thumb>
    <fanart>
        <thumb preview="fanart-small.jpg">fanart.jpg</thumb>
    </fanart>
    <mpaa>R</mpaa>
    <uniqueid type="tmdb" default="true">603</uniqueid>
    <uniqueid type="imdb">tt0133093</uniqueid>
    <genre>Action</genre>
    <genre>Science Fiction</genre>
    <set>
        <name>The Matrix Collection</name>
        <overview>Neo's journey.</overview>
    </set>
    <country>USA</country>
    <credits>Lana Wachowski</credits>
    <director>Lana Wachowski</director>
    <director>Lilly Wachowski</director>
    <premiered>1999-03-31</premiered>
    <studio>Warner Bros.</studio>
    <fileinfo>
        <streamdetails>
            <video>
                <codec>h264</codec>
                <width>1920</width>
                <height>1080</height>
            </video>
            <audio>
                <codec>dts</codec>
                <channels>6</channels>
            </audio>
        </streamdetails>
    </fileinfo>
    <actor>
        <name>Keanu Reeves</name>
        <role>Neo</role>
    </actor>
</movie>`).toMetadata()

	if md.Kind != metadata.KindMovie {
		t.Fatalf("Kind = %q, want %q", md.Kind, metadata.KindMovie)
	}
	if md.Title != "The Matrix" || md.Movie == nil || md.Movie.SortTitle != "Matrix, The" {
		t.Errorf("title/movie fields = %+v / %+v", md.Title, md.Movie)
	}
	if runtime, ok := metadata.RuntimeMinutes(md); !ok || runtime != 136 {
		t.Errorf("RuntimeMinutes() = %v, %v; want 136, true", runtime, ok)
	}
	if len(md.Genres) != 2 || md.Genres[1] != "Science Fiction" {
		t.Errorf("Genres = %v", md.Genres)
	}
	if md.CastCrew == nil || len(md.CastCrew.Directors) != 2 || len(md.CastCrew.Actors) != 1 {
		t.Errorf("CastCrew = %+v", md.CastCrew)
	}
	// <uniqueid> tags read straight into the external links, carrying the
	// default flag with them.
	if len(md.ExternalLinks) != 2 || md.ExternalLinks[0].Key != "tmdb" ||
		md.ExternalLinks[0].Value != "603" || !md.ExternalLinks[0].Default {
		t.Errorf("ExternalLinks = %+v", md.ExternalLinks)
	}
	if md.Ratings == nil || len(md.Ratings.Rating) != 1 || md.Ratings.Rating[0].Votes != "24601" {
		t.Errorf("Ratings = %+v", md.Ratings)
	}
	if md.Movie.Set == nil || md.Movie.Set.Name != "The Matrix Collection" {
		t.Errorf("Set = %+v", md.Movie.Set)
	}
	if md.Movie.Fanart == nil || len(md.Movie.Fanart.Thumbs) != 1 {
		t.Errorf("Fanart = %+v", md.Movie.Fanart)
	}
	if got := metadata.FormatDate(md.Premiered); got != "1999-03-31" {
		t.Errorf("Premiered = %q, want %q", got, "1999-03-31")
	}
	// <fileinfo> is no longer modelled, but NFO files are the system of record,
	// so it has to survive as an unknown element rather than being dropped on
	// the next write.
	if !hasUnknownElement(md.Extra, "fileinfo") {
		t.Errorf("fileinfo was lost instead of being preserved as an unknown element: %+v", md.Extra)
	}
}

func hasUnknownElement(elements []*metadata.UnknownElement, name string) bool {
	for _, element := range elements {
		if element.Name == name {
			return true
		}
	}
	return false
}

func TestParseTVShow(t *testing.T) {
	md := mustParse(t, `<tvshow>
    <title>Breaking Bad</title>
    <plot>A teacher turns to crime.</plot>
    <uniqueid type="tvdb" default="true">81189</uniqueid>
    <status>Ended</status>
    <premiered>2008-01-20</premiered>
    <thumb aspect="poster" type="season" season="1">season01.jpg</thumb>
    <studio>AMC</studio>
</tvshow>`).toMetadata()

	if md.Kind != metadata.KindTVShow {
		t.Fatalf("Kind = %q, want %q", md.Kind, metadata.KindTVShow)
	}
	if md.Title != "Breaking Bad" || md.Status != "Ended" || md.TvShow == nil {
		t.Errorf("tvshow = %+v", md)
	}
	if len(md.Thumbs) != 1 || md.Thumbs[0].Type != "season" || md.Thumbs[0].Season != "1" {
		t.Errorf("Thumbs = %+v", md.Thumbs)
	}
}

func TestParseMusicVideo(t *testing.T) {
	md := mustParse(t, `<musicvideo>
    <title>Take On Me</title>
    <artist>a-ha</artist>
    <artist>John Doe</artist>
    <album>Hunting High and Low</album>
    <track>1</track>
</musicvideo>`).toMetadata()

	if md.Kind != metadata.KindMusicVideo || md.MusicVideo == nil {
		t.Fatalf("md = %+v", md)
	}
	if got := md.MusicVideo.Artists; len(got) != 2 || got[0] != "a-ha" {
		t.Errorf("Artists = %v", got)
	}
	if track, ok := metadata.TrackNumber(md); !ok || track != 1 {
		t.Errorf("TrackNumber() = %v, %v", track, ok)
	}
}

func TestParseSingleEpisode(t *testing.T) {
	md := mustParse(t, `<episodedetails>
    <title>Pilot</title>
    <season>1</season>
    <episode>1</episode>
    <uniqueid type="tvdb" default="true">349232</uniqueid>
    <aired>2008-01-20</aired>
</episodedetails>`).toMetadata()

	if md.Kind != metadata.KindEpisode || md.Episode == nil {
		t.Fatalf("md = %+v", md)
	}
	if season, ok := metadata.SeasonNumber(md); !ok || season != 1 {
		t.Errorf("SeasonNumber() = %v, %v", season, ok)
	}
	if number, ok := metadata.EpisodeNumber(md); !ok || number != 1 {
		t.Errorf("EpisodeNumber() = %v, %v", number, ok)
	}
}

// TestParseMultipleEpisodes covers the Kodi v21-and-earlier multi-episode
// layout: several <episodedetails> roots concatenated into one file. That has
// no single root element, so it is not well-formed XML and xml.Unmarshal
// rejects it outright. The token loop still recovers every episode; toMetadata
// then represents the first.
func TestParseMultipleEpisodes(t *testing.T) {
	doc := mustParse(t, `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>
<episodedetails><title>Episode 1</title><episode>1</episode></episodedetails>
<episodedetails><title>Episode 2</title><episode>2</episode></episodedetails>
<episodedetails><title>Episode 3</title><episode>3</episode></episodedetails>`)

	if doc.Kind != metadata.KindEpisode || len(doc.Episodes) != 3 {
		t.Fatalf("Kind=%q len(Episodes)=%d", doc.Kind, len(doc.Episodes))
	}
	if doc.Episodes[2].Title != "Episode 3" {
		t.Errorf("Episodes[2].Title = %q", doc.Episodes[2].Title)
	}
	if md := doc.toMetadata(); md.Title != "Episode 1" {
		t.Errorf("toMetadata() represents %q, want the first episode", md.Title)
	}
}

// TestParseEmptyNumericTags covers why the numeric-looking fields are strings:
// encoding/xml hard-errors unmarshaling "" into an int, and empty tags are
// commonplace in real files.
func TestParseEmptyNumericTags(t *testing.T) {
	md := mustParse(t, `<movie><title>Sparse</title><runtime></runtime><userrating></userrating><playcount></playcount></movie>`).toMetadata()
	if md.Title != "Sparse" {
		t.Errorf("Title = %q", md.Title)
	}
	if _, ok := metadata.RuntimeMinutes(md); ok {
		t.Error("RuntimeMinutes() reported a value for an empty tag")
	}
	if md.PlayCount != 0 {
		t.Errorf("PlayCount = %d, want 0 for an empty tag", md.PlayCount)
	}
}

func TestParseNonNumericRuntime(t *testing.T) {
	md := mustParse(t, `<movie><title>x</title><runtime>136 min</runtime></movie>`).toMetadata()
	if _, ok := metadata.RuntimeMinutes(md); ok {
		t.Error("RuntimeMinutes() parsed a non-numeric runtime")
	}
	if md.Runtime != "136 min" {
		t.Errorf("Runtime = %q; the raw text must survive verbatim", md.Runtime)
	}
}

// TestParseLegacyEncodings covers files written before UTF-8 was universal.
func TestParseLegacyEncodings(t *testing.T) {
	tests := []struct {
		name      string
		declared  string
		raw       []byte
		wantTitle string
	}{
		{"iso-8859-1 accented", "ISO-8859-1", []byte{'A', 'm', 0xE9, 'l', 'i', 'e'}, "Amélie"},
		{"windows-1252 curly quotes", "windows-1252", []byte{0x93, 'H', 'i', 0x94}, "“Hi”"},
		{"iso-8859-1 label with 1252 bytes", "iso-8859-1", []byte{'C', 'a', 'f', 0xE9, 0x92, 's'}, "Café’s"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var data []byte
			data = append(data, []byte(`<?xml version="1.0" encoding="`+test.declared+`"?><movie><title>`)...)
			data = append(data, test.raw...)
			data = append(data, []byte(`</title></movie>`)...)

			md := mustParse(t, string(data)).toMetadata()
			if md.Title != test.wantTitle {
				t.Errorf("Title = %q, want %q", md.Title, test.wantTitle)
			}
		})
	}
}

func TestParseUnsupportedEncoding(t *testing.T) {
	if _, err := parse([]byte(`<?xml version="1.0" encoding="Shift_JIS"?><movie><title>x</title></movie>`)); err == nil {
		t.Fatal("parse() succeeded on an unsupported encoding; want an error the scanner can record")
	}
}

func TestParseStripsBOM(t *testing.T) {
	md := mustParse(t, string(append([]byte{0xEF, 0xBB, 0xBF}, []byte(`<movie><title>BOM</title></movie>`)...))).toMetadata()
	if md.Kind != metadata.KindMovie || md.Title != "BOM" {
		t.Errorf("md = %+v", md)
	}
}

// TestParseShapes covers the files that are not media documents. These must be
// classified rather than errored, so a single odd file can't fail a scan.
func TestParseShapes(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		wantKind string
	}{
		{"scraper url only", "https://www.themoviedb.org/movie/603\n", metadata.KindURL},
		{"bare identifier", "tt0133093", metadata.KindURL},
		{"empty file", "", metadata.KindUnknown},
		{"whitespace only", "   \n\t ", metadata.KindUnknown},
		{"unrecognized root", `<somethingelse><title>x</title></somethingelse>`, metadata.KindUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := parse([]byte(test.data))
			if err != nil {
				t.Fatalf("parse() error = %v; unrecognized shapes must not error", err)
			}
			if doc.Kind != test.wantKind {
				t.Errorf("Kind = %q, want %q", doc.Kind, test.wantKind)
			}
			if md := doc.toMetadata(); md.Kind != test.wantKind {
				t.Errorf("toMetadata Kind = %q, want %q", md.Kind, test.wantKind)
			}
		})
	}
}

func TestParseMalformedXMLErrors(t *testing.T) {
	if _, err := parse([]byte(`<movie><title>Unclosed`)); err == nil {
		t.Fatal("parse() succeeded on truncated XML; want an error")
	}
}

func TestReadFileMissing(t *testing.T) {
	if _, err := ReadFile("/nonexistent/path/movie.nfo"); err == nil {
		t.Fatal("ReadFile() succeeded for a missing file")
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/movie.nfo"
	if err := writeTestFile(t, path, `<movie><title>From Disk</title></movie>`); err != nil {
		t.Fatal(err)
	}

	md, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if md.Kind != metadata.KindMovie || md.Title != "From Disk" {
		t.Errorf("md = %+v", md)
	}
}

func TestParseHTMLEntitiesInPlot(t *testing.T) {
	// Kodi writes bare HTML entities into plot text; the decoder must not
	// discard the whole document over one of them.
	md := mustParse(t, `<movie><title>Ok</title><plot>a&nbsp;b</plot></movie>`).toMetadata()
	if !strings.Contains(md.Plot, "a") || !strings.Contains(md.Plot, "b") {
		t.Errorf("Plot = %q", md.Plot)
	}
}
