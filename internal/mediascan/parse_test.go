package mediascan

import (
	"reflect"
	"testing"
)

func TestParseVideoName(t *testing.T) {
	tests := []struct {
		name          string
		baseName      string
		folderName    string
		allowBareYear bool
		want          videoNameParts
	}{
		{
			name:          "title and parenthesized year",
			baseName:      "The Matrix (1999)",
			folderName:    "The Matrix (1999)",
			allowBareYear: true,
			want:          videoNameParts{Title: "The Matrix", Year: 1999},
		},
		{
			name:          "provider id tag is stripped but not recorded",
			baseName:      "Movie (2021) [imdbid-tt12801262]",
			folderName:    "Movie (2021) [imdbid-tt12801262]",
			allowBareYear: true,
			want:          videoNameParts{Title: "Movie", Year: 2021},
		},
		{
			name:          "plex brace provider id is stripped",
			baseName:      "Movie (2021) {tmdb-603}",
			folderName:    "Movie (2021) {tmdb-603}",
			allowBareYear: true,
			want:          videoNameParts{Title: "Movie", Year: 2021},
		},
		{
			name:          "plex edition tag",
			baseName:      "Blade Runner (1982) {edition-Director's Cut}",
			folderName:    "Blade Runner (1982)",
			allowBareYear: true,
			want:          videoNameParts{Title: "Blade Runner", Year: 1982, Edition: "Director's Cut"},
		},
		{
			name:          "jellyfin version label matching the folder",
			baseName:      "The Matrix (1999) - 2160p",
			folderName:    "The Matrix (1999)",
			allowBareYear: true,
			want:          videoNameParts{Title: "The Matrix", Year: 1999, VersionLabel: "2160p"},
		},
		{
			name:          "bracketed version label",
			baseName:      "The Matrix (1999) - [Directors Cut]",
			folderName:    "The Matrix (1999)",
			allowBareYear: true,
			want:          videoNameParts{Title: "The Matrix", Year: 1999, VersionLabel: "Directors Cut"},
		},
		{
			// The version-label rule is anchored to the folder name for exactly
			// this reason: a hyphenated title must survive intact.
			name:          "hyphenated title is not a version label",
			baseName:      "Mission - Impossible (1996)",
			folderName:    "Mission - Impossible (1996)",
			allowBareYear: true,
			want:          videoNameParts{Title: "Mission - Impossible", Year: 1996},
		},
		{
			name:          "stack part with dash",
			baseName:      "The Matrix (1999)-cd1",
			folderName:    "The Matrix (1999)",
			allowBareYear: true,
			want:          videoNameParts{Title: "The Matrix", Year: 1999, StackType: "cd", StackNumber: 1},
		},
		{
			name:          "stack part spelled out with a space",
			baseName:      "Series S02E03 Part 2",
			folderName:    "Series",
			allowBareYear: false,
			want:          videoNameParts{Title: "Series S02E03", StackType: "part", StackNumber: 2},
		},
		{
			name:          "three dimensional flag",
			baseName:      "Awesome 3D Movie (2022).3D.FTAB",
			folderName:    "Awesome 3D Movie (2022)",
			allowBareYear: true,
			want:          videoNameParts{Title: "Awesome 3D Movie", Year: 2022, ThreeDFormat: "ftab"},
		},
		{
			name:          "scene naming with bare year",
			baseName:      "Movie.Name.2019.1080p.BluRay.x264",
			folderName:    "Movie.Name.2019",
			allowBareYear: true,
			want:          videoNameParts{Title: "Movie Name", Year: 2019},
		},
		{
			// A movie really can be named for a year; consuming it would leave
			// no title at all.
			name:          "title that is only a year keeps its title",
			baseName:      "1917",
			folderName:    "1917",
			allowBareYear: true,
			want:          videoNameParts{Title: "1917"},
		},
		{
			name:          "numeric title with a real year",
			baseName:      "2012 (2009)",
			folderName:    "2012 (2009)",
			allowBareYear: true,
			want:          videoNameParts{Title: "2012", Year: 2009},
		},
		{
			name:          "trailing release info in brackets is dropped",
			baseName:      "Movie (1999) [1080p BluRay]",
			folderName:    "Movie (1999)",
			allowBareYear: true,
			want:          videoNameParts{Title: "Movie", Year: 1999},
		},
		{
			name:          "tv does not consume a bare year",
			baseName:      "Show 2011",
			folderName:    "Show",
			allowBareYear: false,
			want:          videoNameParts{Title: "Show 2011"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseVideoName(test.baseName, test.folderName, test.allowBareYear)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("parseVideoName(%q, %q, %v)\n got: %+v\nwant: %+v",
					test.baseName, test.folderName, test.allowBareYear, got, test.want)
			}
		})
	}
}

func intPtr(value int) *int { return &value }

func TestParseEpisodeName(t *testing.T) {
	tests := []struct {
		name             string
		baseName         string
		wantSeason       *int
		wantEpisodes     []int
		wantAirDate      string
		wantSeriesTitle  string
		wantEpisodeTitle string
		wantMatched      bool
	}{
		{
			name:            "standard numbering",
			baseName:        "Series Name A S01E01",
			wantSeason:      intPtr(1),
			wantEpisodes:    []int{1},
			wantSeriesTitle: "Series Name A",
			wantMatched:     true,
		},
		{
			name:             "with episode title",
			baseName:         "Breaking Bad S01E01 - Pilot",
			wantSeason:       intPtr(1),
			wantEpisodes:     []int{1},
			wantSeriesTitle:  "Breaking Bad",
			wantEpisodeTitle: "Pilot",
			wantMatched:      true,
		},
		{
			name:            "two episode range",
			baseName:        "Series S01E01-E02",
			wantSeason:      intPtr(1),
			wantEpisodes:    []int{1, 2},
			wantSeriesTitle: "Series",
			wantMatched:     true,
		},
		{
			// A hyphenated span covers the episodes in between, not just its
			// endpoints.
			name:            "three episode range expands",
			baseName:        "Series S01E01-E03",
			wantSeason:      intPtr(1),
			wantEpisodes:    []int{1, 2, 3},
			wantSeriesTitle: "Series",
			wantMatched:     true,
		},
		{
			// Without a hyphen these are an explicit list, so they must not be
			// expanded into a span.
			name:            "adjacent episodes without a hyphen",
			baseName:        "Series S01E01E05",
			wantSeason:      intPtr(1),
			wantEpisodes:    []int{1, 5},
			wantSeriesTitle: "Series",
			wantMatched:     true,
		},
		{
			name:            "specials season zero",
			baseName:        "Series S00E01",
			wantSeason:      intPtr(0),
			wantEpisodes:    []int{1},
			wantSeriesTitle: "Series",
			wantMatched:     true,
		},
		{
			name:            "lowercase and dotted",
			baseName:        "Series.Name.s02e07.1080p",
			wantSeason:      intPtr(2),
			wantEpisodes:    []int{7},
			wantSeriesTitle: "Series Name",
			// Release info trails the episode token here.
			wantEpisodeTitle: "1080p",
			wantMatched:      true,
		},
		{
			name:            "cross notation",
			baseName:        "Series 1x05",
			wantSeason:      intPtr(1),
			wantEpisodes:    []int{5},
			wantSeriesTitle: "Series",
			wantMatched:     true,
		},
		{
			name:            "iso date based",
			baseName:        "ShowName 2011-11-15",
			wantAirDate:     "2011-11-15",
			wantSeriesTitle: "ShowName",
			wantMatched:     true,
		},
		{
			name:            "plex day first date based",
			baseName:        "ShowName 15-11-2011",
			wantAirDate:     "2011-11-15",
			wantSeriesTitle: "ShowName",
			wantMatched:     true,
		},
		{
			name:             "date based with title",
			baseName:         "ShowName 2011-11-15 - Guest Host",
			wantAirDate:      "2011-11-15",
			wantSeriesTitle:  "ShowName",
			wantEpisodeTitle: "Guest Host",
			wantMatched:      true,
		},
		{
			// Absolute numbering, the anime convention, is not resolvable to a
			// season and episode; the caller warns and records the file anyway.
			name:        "absolute numbering does not match",
			baseName:    "Series Name - 052",
			wantMatched: false,
		},
		{
			name:        "no numbering at all",
			baseName:    "Just A Title",
			wantMatched: false,
		},
		{
			name:        "implausible date is rejected",
			baseName:    "Show 2011-45-99",
			wantMatched: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseEpisodeName(test.baseName)

			if got.Matched != test.wantMatched {
				t.Fatalf("Matched = %v, want %v (got %+v)", got.Matched, test.wantMatched, got)
			}
			if !test.wantMatched {
				return
			}
			if !reflect.DeepEqual(got.SeasonNumber, test.wantSeason) {
				t.Errorf("SeasonNumber = %v, want %v", derefSeason(got.SeasonNumber), derefSeason(test.wantSeason))
			}
			if !reflect.DeepEqual(got.EpisodeNumbers, test.wantEpisodes) {
				t.Errorf("EpisodeNumbers = %v, want %v", got.EpisodeNumbers, test.wantEpisodes)
			}
			if got.AirDate != test.wantAirDate {
				t.Errorf("AirDate = %q, want %q", got.AirDate, test.wantAirDate)
			}
			if got.SeriesTitle != test.wantSeriesTitle {
				t.Errorf("SeriesTitle = %q, want %q", got.SeriesTitle, test.wantSeriesTitle)
			}
			if got.EpisodeTitle != test.wantEpisodeTitle {
				t.Errorf("EpisodeTitle = %q, want %q", got.EpisodeTitle, test.wantEpisodeTitle)
			}
		})
	}
}

func derefSeason(season *int) any {
	if season == nil {
		return nil
	}
	return *season
}

func TestParseSeasonFolder(t *testing.T) {
	tests := []struct {
		folderName      string
		wantSeason      int
		wantAbbreviated bool
		wantOK          bool
	}{
		{"Season 01", 1, false, true},
		{"Season 1", 1, false, true},
		{"season 12", 12, false, true},
		{"Season 00", 0, false, true},
		{"Specials", 0, false, true},
		{"specials", 0, false, true},
		// Jellyfin forbids these, but a library using them should still scan.
		{"S01", 1, true, true},
		{"SE01", 1, true, true},
		{"Behind The Scenes", 0, false, false},
		{"Extras", 0, false, false},
		{"", 0, false, false},
	}

	for _, test := range tests {
		t.Run(test.folderName, func(t *testing.T) {
			season, abbreviated, ok := parseSeasonFolder(test.folderName)
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if !ok {
				return
			}
			if season != test.wantSeason {
				t.Errorf("season = %d, want %d", season, test.wantSeason)
			}
			if abbreviated != test.wantAbbreviated {
				t.Errorf("abbreviated = %v, want %v", abbreviated, test.wantAbbreviated)
			}
		})
	}
}

func TestExtrasSuffixType(t *testing.T) {
	tests := []struct {
		baseName string
		want     string
		wantOK   bool
	}{
		{"Movie-trailer", ExtraTrailer, true},
		{"Movie.trailer", ExtraTrailer, true},
		{"Movie_trailer", ExtraTrailer, true},
		{"Movie trailer", ExtraTrailer, true},
		{"Movie-sample", ExtraSample, true},
		{"Movie-behindthescenes", ExtraBehindTheScenes, true},
		{"Movie-deleted", ExtraDeletedScene, true},
		{"Movie-deletedscene", ExtraDeletedScene, true},
		{"Movie-featurette", ExtraFeaturette, true},
		{"Movie-interview", ExtraInterview, true},
		{"Movie-short", ExtraShort, true},
		{"Movie-other", ExtraOther, true},
		{"Movie-extra", ExtraExtra, true},
		// Only "trailer" and "sample" may be introduced by a space. Otherwise a
		// feature legitimately titled "The Other" or "The Short" would be
		// demoted to an extra and vanish from the media files.
		{"The Other", "", false},
		{"The Short", "", false},
		{"The Scene", "", false},
		{"The Matrix (1999)", "", false},
	}

	for _, test := range tests {
		t.Run(test.baseName, func(t *testing.T) {
			got, ok := extrasSuffixType(test.baseName)
			if ok != test.wantOK || got != test.want {
				t.Errorf("extrasSuffixType(%q) = %q, %v; want %q, %v", test.baseName, got, ok, test.want, test.wantOK)
			}
		})
	}
}

func TestParseArtistAndTitle(t *testing.T) {
	tests := []struct {
		baseName   string
		wantArtist string
		wantTitle  string
	}{
		{"a-ha - Take On Me", "a-ha", "Take On Me"},
		{"Artist Name - Song Title", "Artist Name", "Song Title"},
		{"JustATitle", "", "JustATitle"},
	}

	for _, test := range tests {
		t.Run(test.baseName, func(t *testing.T) {
			artist, title := parseArtistAndTitle(test.baseName)
			if artist != test.wantArtist || title != test.wantTitle {
				t.Errorf("parseArtistAndTitle(%q) = %q, %q; want %q, %q",
					test.baseName, artist, title, test.wantArtist, test.wantTitle)
			}
		})
	}
}

func TestParseDirectoryType(t *testing.T) {
	for _, valid := range []string{"movie", "tv", "music_video"} {
		if _, err := ParseDirectoryType(valid); err != nil {
			t.Errorf("ParseDirectoryType(%q) error = %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "anime", "Movie", "tvshow"} {
		if _, err := ParseDirectoryType(invalid); err == nil {
			t.Errorf("ParseDirectoryType(%q) succeeded, want an error", invalid)
		}
	}
}

func TestClassifyExtension(t *testing.T) {
	tests := []struct {
		extension string
		want      fileClass
	}{
		{"mkv", classVideo},
		{"mp4", classVideo},
		{"srt", classSubtitle},
		{"jpg", classImage},
		{"tbn", classImage},
		{"nfo", classNFO},
		{"mp3", classAudio},
		{"txt", classOther},
		{"", classOther},
	}

	for _, test := range tests {
		if got := classifyExtension(test.extension); got != test.want {
			t.Errorf("classifyExtension(%q) = %v, want %v", test.extension, got, test.want)
		}
	}
}
