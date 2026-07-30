package nfo

import (
	"strings"
	"testing"
)

func TestParseMovie(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>
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
                <durationinseconds>8160</durationinseconds>
                <hdrtype>hdr10</hdrtype>
            </video>
            <audio>
                <codec>dts</codec>
                <language>eng</language>
                <channels>6</channels>
            </audio>
            <subtitle>
                <language>eng</language>
            </subtitle>
        </streamdetails>
    </fileinfo>
    <actor>
        <name>Keanu Reeves</name>
        <role>Neo</role>
        <order>0</order>
        <thumb>keanu.jpg</thumb>
    </actor>
</movie>`)

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.Kind != KindMovie {
		t.Fatalf("Kind = %q, want %q", doc.Kind, KindMovie)
	}
	movie := doc.Movie
	if movie == nil {
		t.Fatal("Movie is nil")
	}

	if movie.Title != "The Matrix" {
		t.Errorf("Title = %q", movie.Title)
	}
	if movie.SortTitle != "Matrix, The" {
		t.Errorf("SortTitle = %q", movie.SortTitle)
	}
	if runtime, ok := movie.RuntimeMinutes(); !ok || runtime != 136 {
		t.Errorf("RuntimeMinutes() = %v, %v; want 136, true", runtime, ok)
	}
	if len(movie.Genres) != 2 || movie.Genres[1] != "Science Fiction" {
		t.Errorf("Genres = %v", movie.Genres)
	}
	if len(movie.Directors) != 2 {
		t.Errorf("Directors = %v", movie.Directors)
	}
	if len(movie.UniqueIDs) != 2 {
		t.Fatalf("UniqueIDs = %v", movie.UniqueIDs)
	}
	if movie.UniqueIDs[0].Type != "tmdb" || movie.UniqueIDs[0].Value != "603" || !movie.UniqueIDs[0].Default {
		t.Errorf("UniqueIDs[0] = %+v", movie.UniqueIDs[0])
	}
	if movie.Ratings == nil || len(movie.Ratings.Rating) != 1 {
		t.Fatalf("Ratings = %+v", movie.Ratings)
	}
	if rating := movie.Ratings.Rating[0]; rating.Name != "themoviedb" || rating.Value != "8.2" || rating.Votes != "24601" {
		t.Errorf("Rating = %+v", rating)
	}
	if movie.Set == nil || movie.Set.Name != "The Matrix Collection" {
		t.Errorf("Set = %+v", movie.Set)
	}
	if len(movie.Thumbs) != 1 || movie.Thumbs[0].Aspect != "poster" || movie.Thumbs[0].Value != "poster.jpg" {
		t.Errorf("Thumbs = %+v", movie.Thumbs)
	}
	if movie.Fanart == nil || len(movie.Fanart.Thumbs) != 1 || movie.Fanart.Thumbs[0].Value != "fanart.jpg" {
		t.Errorf("Fanart = %+v", movie.Fanart)
	}
	streams := movie.FileInfo.StreamDetails
	if len(streams.Video) != 1 || streams.Video[0].Height != "1080" || streams.Video[0].HDRType != "hdr10" {
		t.Errorf("Video streams = %+v", streams.Video)
	}
	if len(streams.Audio) != 1 || streams.Audio[0].Channels != "6" {
		t.Errorf("Audio streams = %+v", streams.Audio)
	}
	if len(movie.Actors) != 1 || movie.Actors[0].Role != "Neo" {
		t.Errorf("Actors = %+v", movie.Actors)
	}
}

func TestParseTVShow(t *testing.T) {
	data := []byte(`<tvshow>
    <title>Breaking Bad</title>
    <plot>A teacher turns to crime.</plot>
    <uniqueid type="tvdb" default="true">81189</uniqueid>
    <episodeguide>{"tvdb":"81189"}</episodeguide>
    <status>Ended</status>
    <premiered>2008-01-20</premiered>
    <namedseason number="1">Season One</namedseason>
    <namedseason number="2">Season Two</namedseason>
    <seasonplot number="1">The beginning.</seasonplot>
    <thumb aspect="poster" type="season" season="1">season01.jpg</thumb>
    <studio>AMC</studio>
</tvshow>`)

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.Kind != KindTVShow {
		t.Fatalf("Kind = %q, want %q", doc.Kind, KindTVShow)
	}
	show := doc.TVShow
	if show.Title != "Breaking Bad" || show.Status != "Ended" {
		t.Errorf("show = %+v", show)
	}
	if len(show.NamedSeasons) != 2 || show.NamedSeasons[1].Number != "2" || show.NamedSeasons[1].Value != "Season Two" {
		t.Errorf("NamedSeasons = %+v", show.NamedSeasons)
	}
	if len(show.SeasonPlots) != 1 || show.SeasonPlots[0].Number != "1" {
		t.Errorf("SeasonPlots = %+v", show.SeasonPlots)
	}
	if len(show.Thumbs) != 1 || show.Thumbs[0].Type != "season" || show.Thumbs[0].Season != "1" {
		t.Errorf("Thumbs = %+v", show.Thumbs)
	}
}

func TestParseMusicVideo(t *testing.T) {
	data := []byte(`<musicvideo>
    <title>Take On Me</title>
    <artist>a-ha</artist>
    <artist>John Doe</artist>
    <album>Hunting High and Low</album>
    <track>1</track>
    <director>Steve Barron</director>
    <premiered>1985-09-16</premiered>
</musicvideo>`)

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.Kind != KindMusicVideo {
		t.Fatalf("Kind = %q", doc.Kind)
	}
	if got := doc.MusicVideo.Artists; len(got) != 2 || got[0] != "a-ha" {
		t.Errorf("Artists = %v", got)
	}
	if track, ok := doc.MusicVideo.TrackNumber(); !ok || track != 1 {
		t.Errorf("TrackNumber() = %v, %v", track, ok)
	}
}

// TestParseSingleEpisode and TestParseMultipleEpisodes together cover the
// reason this package walks tokens instead of calling xml.Unmarshal.
func TestParseSingleEpisode(t *testing.T) {
	data := []byte(`<episodedetails>
    <title>Pilot</title>
    <season>1</season>
    <episode>1</episode>
    <uniqueid type="tvdb" default="true">349232</uniqueid>
    <aired>2008-01-20</aired>
    <actor><name>Bryan Cranston</name><role>Walter White</role></actor>
</episodedetails>`)

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.Kind != KindEpisode {
		t.Fatalf("Kind = %q", doc.Kind)
	}
	if len(doc.Episodes) != 1 {
		t.Fatalf("len(Episodes) = %d, want 1", len(doc.Episodes))
	}
	episode := doc.Episodes[0]
	if season, ok := episode.SeasonNumber(); !ok || season != 1 {
		t.Errorf("SeasonNumber() = %v, %v", season, ok)
	}
	if number, ok := episode.EpisodeNumber(); !ok || number != 1 {
		t.Errorf("EpisodeNumber() = %v, %v", number, ok)
	}
}

// TestParseMultipleEpisodes covers the Kodi v21-and-earlier multi-episode
// layout: several <episodedetails> roots concatenated into one file. That has
// no single root element, so it is not well-formed XML and xml.Unmarshal
// rejects it outright — every episode's metadata would be lost.
func TestParseMultipleEpisodes(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>
<episodedetails>
    <title>Episode 1</title>
    <season>1</season>
    <episode>1</episode>
</episodedetails>
<episodedetails>
    <title>Episode 2</title>
    <season>1</season>
    <episode>2</episode>
</episodedetails>
<episodedetails>
    <title>Episode 3</title>
    <season>1</season>
    <episode>3</episode>
</episodedetails>`)

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.Kind != KindEpisode {
		t.Fatalf("Kind = %q", doc.Kind)
	}
	if len(doc.Episodes) != 3 {
		t.Fatalf("len(Episodes) = %d, want 3", len(doc.Episodes))
	}
	for i, episode := range doc.Episodes {
		wantTitle := []string{"Episode 1", "Episode 2", "Episode 3"}[i]
		if episode.Title != wantTitle {
			t.Errorf("Episodes[%d].Title = %q, want %q", i, episode.Title, wantTitle)
		}
		if number, ok := episode.EpisodeNumber(); !ok || number != i+1 {
			t.Errorf("Episodes[%d].EpisodeNumber() = %v, %v; want %d", i, number, ok, i+1)
		}
	}
}

// TestParseEmptyNumericTags covers why the numeric-looking fields are strings:
// encoding/xml hard-errors unmarshaling "" into an int, and empty tags are
// commonplace in real files.
func TestParseEmptyNumericTags(t *testing.T) {
	data := []byte(`<movie>
    <title>Sparse</title>
    <runtime></runtime>
    <userrating></userrating>
    <top250></top250>
    <playcount></playcount>
</movie>`)

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.Movie.Title != "Sparse" {
		t.Errorf("Title = %q", doc.Movie.Title)
	}
	if _, ok := doc.Movie.RuntimeMinutes(); ok {
		t.Error("RuntimeMinutes() reported a value for an empty tag")
	}
	if _, ok := doc.Movie.UserRatingValue(); ok {
		t.Error("UserRatingValue() reported a value for an empty tag")
	}
}

func TestParseNonNumericRuntime(t *testing.T) {
	doc, err := Parse([]byte(`<movie><title>x</title><runtime>136 min</runtime></movie>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if _, ok := doc.Movie.RuntimeMinutes(); ok {
		t.Error("RuntimeMinutes() parsed a non-numeric runtime")
	}
	if doc.Movie.Runtime != "136 min" {
		t.Errorf("Runtime = %q; the raw text must survive verbatim", doc.Movie.Runtime)
	}
}

// TestParseLegacyEncodings covers files written before UTF-8 was universal.
// encoding/xml refuses a declared encoding it doesn't know, so without a
// CharsetReader these would fail to parse at all.
func TestParseLegacyEncodings(t *testing.T) {
	tests := []struct {
		name     string
		declared string
		// 0xE9 is é in both ISO-8859-1 and Windows-1252; 0x93/0x94 are curly
		// quotes in Windows-1252 only.
		raw       []byte
		wantTitle string
	}{
		{
			name:      "iso-8859-1 accented",
			declared:  "ISO-8859-1",
			raw:       []byte{'A', 'm', 0xE9, 'l', 'i', 'e'},
			wantTitle: "Amélie",
		},
		{
			name:      "windows-1252 curly quotes",
			declared:  "windows-1252",
			raw:       []byte{0x93, 'H', 'i', 0x94},
			wantTitle: "“Hi”",
		},
		{
			// Files labelled Latin-1 in practice carry 1252 punctuation; the
			// reader decodes the label as 1252 so this recovers real text.
			name:      "iso-8859-1 label with 1252 bytes",
			declared:  "iso-8859-1",
			raw:       []byte{'C', 'a', 'f', 0xE9, 0x92, 's'},
			wantTitle: "Café’s",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var data []byte
			data = append(data, []byte(`<?xml version="1.0" encoding="`+test.declared+`"?><movie><title>`)...)
			data = append(data, test.raw...)
			data = append(data, []byte(`</title></movie>`)...)

			doc, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if doc.Movie.Title != test.wantTitle {
				t.Errorf("Title = %q, want %q", doc.Movie.Title, test.wantTitle)
			}
		})
	}
}

func TestParseUnsupportedEncoding(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="Shift_JIS"?><movie><title>x</title></movie>`)
	if _, err := Parse(data); err == nil {
		t.Fatal("Parse() succeeded on an unsupported encoding; want an error the scanner can record")
	}
}

func TestParseStripsBOM(t *testing.T) {
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`<movie><title>BOM</title></movie>`)...)
	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.Movie == nil || doc.Movie.Title != "BOM" {
		t.Errorf("doc = %+v", doc)
	}
}

// TestParseShapes covers the files that are not media documents. These must be
// classified rather than errored, so a single odd file can't fail a scan.
func TestParseShapes(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		wantKind DocumentKind
	}{
		{"scraper url only", "https://www.themoviedb.org/movie/603\n", KindURL},
		{"bare identifier", "tt0133093", KindURL},
		{"empty file", "", KindUnknown},
		{"whitespace only", "   \n\t ", KindUnknown},
		{"unrecognized root", `<somethingelse><title>x</title></somethingelse>`, KindUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, err := Parse([]byte(test.data))
			if err != nil {
				t.Fatalf("Parse() error = %v; unrecognized shapes must not error", err)
			}
			if doc.Kind != test.wantKind {
				t.Errorf("Kind = %q, want %q", doc.Kind, test.wantKind)
			}
		})
	}
}

func TestParseMalformedXMLErrors(t *testing.T) {
	// A truncated element is genuinely broken rather than merely unrecognized.
	if _, err := Parse([]byte(`<movie><title>Unclosed`)); err == nil {
		t.Fatal("Parse() succeeded on truncated XML; want an error")
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

	doc, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if doc.Movie == nil || doc.Movie.Title != "From Disk" {
		t.Errorf("doc = %+v", doc)
	}
}

func TestParseHTMLEntitiesInPlot(t *testing.T) {
	// Kodi writes bare HTML entities into plot text; the decoder must not
	// discard the whole document over one of them.
	doc, err := Parse([]byte(`<movie><title>Ok</title><plot>a&nbsp;b</plot></movie>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !strings.Contains(doc.Movie.Plot, "a") || !strings.Contains(doc.Movie.Plot, "b") {
		t.Errorf("Plot = %q", doc.Movie.Plot)
	}
}
