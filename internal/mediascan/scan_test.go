package mediascan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Metarr/internal/nfo"
)

// buildTree materializes a directory tree from a map of relative path to
// content, creating parent folders as needed. Empty content is fine: the scanner
// classifies on names, sizes and modification times, so no real media is
// required to exercise it.
func buildTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()

	for relativePath, content := range files {
		fullPath := filepath.Join(root, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("creating %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("writing %s: %v", fullPath, err)
		}
	}
	return root
}

// scanTree builds a tree inside a named item folder and scans it, which is how
// the scanner is really used: the caller hands it one item directory.
func scanTree(t *testing.T, folderName string, directoryType DirectoryType, files map[string]string) *ScanResult {
	t.Helper()

	prefixed := make(map[string]string, len(files))
	for relativePath, content := range files {
		prefixed[folderName+"/"+relativePath] = content
	}
	root := buildTree(t, prefixed)

	result, err := Scan(filepath.Join(root, folderName), directoryType)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	return result
}

// mediaFileByName finds a media file record by its file name.
func mediaFileByName(t *testing.T, result *ScanResult, fileName string) MediaFile {
	t.Helper()
	for _, mediaFile := range result.MediaFiles {
		if mediaFile.FileName == fileName {
			return mediaFile
		}
	}
	t.Fatalf("no media file record named %q; got %s", fileName, mediaFileNames(result))
	return MediaFile{}
}

func mediaFileNames(result *ScanResult) string {
	names := make([]string, 0, len(result.MediaFiles))
	for _, mediaFile := range result.MediaFiles {
		names = append(names, mediaFile.FileName)
	}
	return "[" + strings.Join(names, ", ") + "]"
}

func directoryFileByName(t *testing.T, result *ScanResult, fileName string) DirectoryFile {
	t.Helper()
	for _, file := range result.Directory.Files {
		if file.FileName == fileName {
			return file
		}
	}
	t.Fatalf("no directory file named %q; got %s", fileName, directoryFileNames(result))
	return DirectoryFile{}
}

func directoryFileNames(result *ScanResult) string {
	names := make([]string, 0, len(result.Directory.Files))
	for _, file := range result.Directory.Files {
		names = append(names, file.FileName+"("+string(file.Role)+")")
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// TestScanMovieRecordSplit is the core assertion of the storage model: the
// feature is a media file record, and everything else stays on the directory.
func TestScanMovieRecordSplit(t *testing.T) {
	result := scanTree(t, "The Matrix (1999)", TypeMovie, map[string]string{
		"The Matrix (1999).mkv":           "",
		"The Matrix (1999).en.srt":        "",
		"The Matrix (1999).nfo":           `<movie><title>The Matrix</title><uniqueid type="tmdb">603</uniqueid></movie>`,
		"poster.jpg":                      "",
		"fanart.jpg":                      "",
		"backdrop-1.jpg":                  "",
		"theme.mp3":                       "",
		"The Matrix (1999)-trailer.mkv":   "",
		"Behind The Scenes/Making Of.mkv": "",
		"Trailers/Teaser.mkv":             "",
	})

	if len(result.MediaFiles) != 1 {
		t.Fatalf("len(MediaFiles) = %d, want 1; got %s", len(result.MediaFiles), mediaFileNames(result))
	}
	feature := result.MediaFiles[0]
	if feature.FileName != "The Matrix (1999).mkv" {
		t.Errorf("media file = %q", feature.FileName)
	}
	if feature.RecordType != RecordTypeMediaFile {
		t.Errorf("RecordType = %q", feature.RecordType)
	}
	if feature.Video == nil || feature.Video.Title != "The Matrix" || feature.Video.Year != 1999 {
		t.Errorf("Video = %+v", feature.Video)
	}

	// The feature's own sidecars ride with it.
	if len(feature.Subtitles) != 1 || feature.Subtitles[0].Subtitle.Language != "en" {
		t.Errorf("Subtitles = %+v", feature.Subtitles)
	}
	if feature.NFO == nil || feature.NFO.Scope != ScopeVideo || feature.NFO.Movie == nil {
		t.Errorf("NFO = %+v", feature.NFO)
	}

	// Extras, artwork and theme music stay on the directory and must not appear
	// as media files.
	for _, name := range []string{
		"poster.jpg", "fanart.jpg", "backdrop-1.jpg", "theme.mp3",
		"The Matrix (1999)-trailer.mkv", "Making Of.mkv", "Teaser.mkv",
	} {
		directoryFileByName(t, result, name)
	}

	if role := directoryFileByName(t, result, "theme.mp3").Role; role != RoleThemeMusic {
		t.Errorf("theme.mp3 role = %q, want %q", role, RoleThemeMusic)
	}

	trailer := directoryFileByName(t, result, "The Matrix (1999)-trailer.mkv")
	if trailer.Role != RoleExtraVideo || trailer.Video.ExtraType != ExtraTrailer || trailer.Video.ExtraSource != ExtraSourceSuffix {
		t.Errorf("suffix trailer = %+v / %+v", trailer.Role, trailer.Video)
	}

	makingOf := directoryFileByName(t, result, "Making Of.mkv")
	if makingOf.Role != RoleExtraVideo || makingOf.Video.ExtraType != ExtraBehindTheScenes || makingOf.Video.ExtraSource != ExtraSourceFolder {
		t.Errorf("folder extra = %+v / %+v", makingOf.Role, makingOf.Video)
	}

	backdrop := directoryFileByName(t, result, "backdrop-1.jpg")
	if backdrop.Image == nil || backdrop.Image.ImageType != ImageBackdrop || backdrop.Image.Index != 1 {
		t.Errorf("backdrop = %+v", backdrop.Image)
	}
}

func TestScanMovieMultipleVersions(t *testing.T) {
	result := scanTree(t, "Blade Runner (1982)", TypeMovie, map[string]string{
		"Blade Runner (1982) - 2160p.mkv":                  "",
		"Blade Runner (1982) {edition-Director's Cut}.mkv": "",
		"Blade Runner (1982)-cd1.mkv":                      "",
		"Blade Runner (1982)-cd2.mkv":                      "",
	})

	if len(result.MediaFiles) != 4 {
		t.Fatalf("len(MediaFiles) = %d, want 4; got %s", len(result.MediaFiles), mediaFileNames(result))
	}

	version := mediaFileByName(t, result, "Blade Runner (1982) - 2160p.mkv")
	if version.Video.VersionLabel != "2160p" {
		t.Errorf("VersionLabel = %q", version.Video.VersionLabel)
	}

	edition := mediaFileByName(t, result, "Blade Runner (1982) {edition-Director's Cut}.mkv")
	if edition.Video.Edition != "Director's Cut" {
		t.Errorf("Edition = %q", edition.Video.Edition)
	}

	part := mediaFileByName(t, result, "Blade Runner (1982)-cd2.mkv")
	if part.Video.StackType != "cd" || part.Video.StackNumber != 2 {
		t.Errorf("stack = %q %d", part.Video.StackType, part.Video.StackNumber)
	}
}

// TestScanSeriesRecordSplit covers the layered case: episodes become records,
// per-episode sidecars attach to them, and series-level files stay put.
func TestScanSeriesRecordSplit(t *testing.T) {
	result := scanTree(t, "Breaking Bad (2008)", TypeTV, map[string]string{
		"tvshow.nfo":                              `<tvshow><title>Breaking Bad</title><uniqueid type="tvdb">81189</uniqueid></tvshow>`,
		"poster.jpg":                              "",
		"Season01-poster.jpg":                     "",
		"theme.mp3":                               "",
		"Season 01/season.nfo":                    `<tvshow><title>Season 1</title></tvshow>`,
		"Season 01/Breaking Bad S01E01.mkv":       "",
		"Season 01/Breaking Bad S01E01.en.srt":    "",
		"Season 01/Breaking Bad S01E01-thumb.jpg": "",
		"Season 01/Breaking Bad S01E01.nfo": `<episodedetails><title>Pilot</title><season>1</season>` +
			`<episode>1</episode><uniqueid type="tvdb">349232</uniqueid></episodedetails>`,
		"Season 01/Breaking Bad S01E02.mkv": "",
		"Season 02/Breaking Bad S02E01.mkv": "",
		"Specials/Breaking Bad S00E01.mkv":  "",
		"Trailers/Season 1 Trailer.mkv":     "",
	})

	if len(result.MediaFiles) != 4 {
		t.Fatalf("len(MediaFiles) = %d, want 4; got %s", len(result.MediaFiles), mediaFileNames(result))
	}

	// Series-level files stay on the directory record.
	for _, name := range []string{"tvshow.nfo", "poster.jpg", "Season01-poster.jpg", "theme.mp3", "season.nfo", "Season 1 Trailer.mkv"} {
		directoryFileByName(t, result, name)
	}

	// The episode's own sidecars attach to the episode, not to the directory.
	episode := mediaFileByName(t, result, "Breaking Bad S01E01.mkv")
	if len(episode.Subtitles) != 1 || episode.Subtitles[0].FileName != "Breaking Bad S01E01.en.srt" {
		t.Errorf("Subtitles = %+v", episode.Subtitles)
	}
	if len(episode.Images) != 1 || episode.Images[0].FileName != "Breaking Bad S01E01-thumb.jpg" {
		t.Errorf("Images = %+v", episode.Images)
	}
	if episode.Images[0].Image.ImageType != ImageThumb || episode.Images[0].Image.Scope != ScopeVideo {
		t.Errorf("episode image = %+v", episode.Images[0].Image)
	}
	if episode.NFO == nil || len(episode.NFO.Episodes) != 1 || episode.NFO.Episodes[0].Title != "Pilot" {
		t.Errorf("NFO = %+v", episode.NFO)
	}
	for _, name := range []string{"Breaking Bad S01E01.en.srt", "Breaking Bad S01E01-thumb.jpg", "Breaking Bad S01E01.nfo"} {
		for _, file := range result.Directory.Files {
			if file.FileName == name {
				t.Errorf("%q should belong to its episode record, not the directory", name)
			}
		}
	}

	if episode.Video.SeasonNumber == nil || *episode.Video.SeasonNumber != 1 {
		t.Errorf("SeasonNumber = %v", derefSeason(episode.Video.SeasonNumber))
	}

	special := mediaFileByName(t, result, "Breaking Bad S00E01.mkv")
	if !special.Video.IsSpecial {
		t.Errorf("season zero episode should be marked special: %+v", special.Video)
	}

	// The season index counts episodes without duplicating them.
	wantSeasons := map[int]int{0: 1, 1: 2, 2: 1}
	if len(result.Directory.Seasons) != len(wantSeasons) {
		t.Fatalf("Seasons = %+v", result.Directory.Seasons)
	}
	for _, season := range result.Directory.Seasons {
		if want := wantSeasons[season.SeasonNumber]; season.EpisodeCount != want {
			t.Errorf("season %d count = %d, want %d", season.SeasonNumber, season.EpisodeCount, want)
		}
	}
	if result.Directory.Seasons[0].SeasonNumber != 0 {
		t.Errorf("Seasons should be ordered by number: %+v", result.Directory.Seasons)
	}
}

// TestScanSeriesLinkSeparation covers the identity split: series ids on the
// directory, episode ids on the episode.
func TestScanSeriesLinkSeparation(t *testing.T) {
	result := scanTree(t, "Breaking Bad (2008)", TypeTV, map[string]string{
		"tvshow.nfo": `<tvshow><title>Breaking Bad</title>` +
			`<uniqueid type="tvdb">81189</uniqueid><uniqueid type="imdb">tt0903747</uniqueid>` +
			`<trailer>plugin://plugin.video.youtube/?action=play_video&amp;videoid=HhesaQXLuRY</trailer></tvshow>`,
		"Season 01/Breaking Bad S01E01.mkv": "",
		"Season 01/Breaking Bad S01E01.nfo": `<episodedetails><title>Pilot</title>` +
			`<uniqueid type="tvdb">349232</uniqueid></episodedetails>`,
		"Season 01/Breaking Bad S01E02.mkv": "",
		"Season 01/Breaking Bad S01E02.nfo": `<episodedetails><title>Cat's in the Bag...</title>` +
			`<uniqueid type="tvdb">349233</uniqueid></episodedetails>`,
	})

	links := result.Directory.ExternalLinks
	if !hasLink(links, nfo.Link{Key: "tvdb", Value: "81189"}) {
		t.Errorf("directory external_links missing the series tvdb id: %+v", links)
	}
	if !hasLink(links, nfo.Link{Key: "imdb", Value: "tt0903747"}) {
		t.Errorf("directory external_links missing the imdb id: %+v", links)
	}
	if !hasLink(links, nfo.Link{Key: "youtube", Value: "HhesaQXLuRY"}) {
		t.Errorf("directory external_links missing the youtube trailer id: %+v", links)
	}

	// Episode ids are deliberately kept out of the directory's links.
	for _, episodeID := range []string{"349232", "349233"} {
		if hasLink(links, nfo.Link{Key: "tvdb", Value: episodeID}) {
			t.Errorf("episode id %s leaked into the directory's external_links: %+v", episodeID, links)
		}
	}

	first := mediaFileByName(t, result, "Breaking Bad S01E01.mkv")
	if !hasLink(first.EpisodeIDs, nfo.Link{Key: "tvdb", Value: "349232"}) {
		t.Errorf("episode 1 episode_ids = %+v", first.EpisodeIDs)
	}
	second := mediaFileByName(t, result, "Breaking Bad S01E02.mkv")
	if !hasLink(second.EpisodeIDs, nfo.Link{Key: "tvdb", Value: "349233"}) {
		t.Errorf("episode 2 episode_ids = %+v", second.EpisodeIDs)
	}
	if hasLink(first.EpisodeIDs, nfo.Link{Key: "tvdb", Value: "349233"}) {
		t.Error("episode ids bled between episodes")
	}
}

func hasLink(links []nfo.Link, want nfo.Link) bool {
	for _, link := range links {
		if link == want {
			return true
		}
	}
	return false
}

// TestScanLegacyMultiEpisodeNFO covers a v21-era sidecar holding several
// episodes, which the NFO reader accepts and whose ids must all be captured.
func TestScanLegacyMultiEpisodeNFO(t *testing.T) {
	result := scanTree(t, "Show (2010)", TypeTV, map[string]string{
		"Season 01/Show S01E01-E02.mkv": "",
		"Season 01/Show S01E01-E02.nfo": `<episodedetails><title>Part 1</title><episode>1</episode>` +
			`<uniqueid type="tvdb">1</uniqueid></episodedetails>` +
			`<episodedetails><title>Part 2</title><episode>2</episode>` +
			`<uniqueid type="tvdb">2</uniqueid></episodedetails>`,
	})

	episode := mediaFileByName(t, result, "Show S01E01-E02.mkv")
	if got := episode.Video.EpisodeNumbers; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("EpisodeNumbers = %v, want [1 2]", got)
	}
	if episode.NFO == nil || len(episode.NFO.Episodes) != 2 {
		t.Fatalf("NFO episodes = %+v", episode.NFO)
	}
	for _, id := range []string{"1", "2"} {
		if !hasLink(episode.EpisodeIDs, nfo.Link{Key: "tvdb", Value: id}) {
			t.Errorf("episode_ids missing tvdb=%s: %+v", id, episode.EpisodeIDs)
		}
	}
}

// TestScanDiscStructure covers a ripped DVD: one item, not one media file per
// VOB.
func TestScanDiscStructure(t *testing.T) {
	result := scanTree(t, "Old Movie (1975)", TypeMovie, map[string]string{
		"VIDEO_TS/VIDEO_TS.IFO": "",
		"VIDEO_TS/VTS_01_0.VOB": "",
		"VIDEO_TS/VTS_01_1.VOB": "",
		"VIDEO_TS/VTS_01_2.VOB": "",
		"poster.jpg":            "",
	})

	if len(result.MediaFiles) != 0 {
		t.Fatalf("len(MediaFiles) = %d, want 0; a disc rip must not become one record per VOB: %s",
			len(result.MediaFiles), mediaFileNames(result))
	}
	for _, name := range []string{"VIDEO_TS.IFO", "VTS_01_0.VOB", "VTS_01_1.VOB", "VTS_01_2.VOB"} {
		if role := directoryFileByName(t, result, name).Role; role != RoleDiscStructure {
			t.Errorf("%s role = %q, want %q", name, role, RoleDiscStructure)
		}
	}
}

func TestScanMusicVideos(t *testing.T) {
	result := scanTree(t, "a-ha", TypeMusicVideo, map[string]string{
		"a-ha - Take On Me.mp4":            "",
		"a-ha - The Sun Always Shines.mp4": "",
		"folder.jpg":                       "",
	})

	if len(result.MediaFiles) != 2 {
		t.Fatalf("len(MediaFiles) = %d, want 2; got %s", len(result.MediaFiles), mediaFileNames(result))
	}
	if result.Directory.Artist != "a-ha" {
		t.Errorf("Artist = %q, want %q", result.Directory.Artist, "a-ha")
	}

	video := mediaFileByName(t, result, "a-ha - Take On Me.mp4")
	if video.Video.Title != "Take On Me" {
		t.Errorf("Title = %q, want %q", video.Video.Title, "Take On Me")
	}
}

func TestScanMusicVideosNested(t *testing.T) {
	// Jellyfin allows arbitrary nesting under a music video library.
	result := scanTree(t, "Various", TypeMusicVideo, map[string]string{
		"Eighties/a-ha - Take On Me.mp4":     "",
		"Nineties/Deep/Some Band - Song.mp4": "",
	})

	if len(result.MediaFiles) != 2 {
		t.Fatalf("len(MediaFiles) = %d, want 2; got %s", len(result.MediaFiles), mediaFileNames(result))
	}
}

// TestScanIgnoresJunk covers the platform and scraper noise that must never
// reach the records.
func TestScanIgnoresJunk(t *testing.T) {
	result := scanTree(t, "Movie (2000)", TypeMovie, map[string]string{
		"Movie (2000).mkv":             "",
		".DS_Store":                    "",
		"Thumbs.db":                    "",
		"._Movie (2000).mkv":           "",
		"extrafanart/fanart1.jpg":      "",
		"extrathumbs/thumb1.jpg":       "",
		".actors/Someone.jpg":          "",
		"Movie (2000).trickplay/x.bif": "",
	})

	if len(result.MediaFiles) != 1 {
		t.Fatalf("len(MediaFiles) = %d, want 1; got %s", len(result.MediaFiles), mediaFileNames(result))
	}
	if len(result.Directory.Files) != 0 {
		t.Errorf("junk reached the directory record: %s", directoryFileNames(result))
	}
}

// TestScanCorruptNFO covers the rule that one bad sidecar cannot fail a scan.
func TestScanCorruptNFO(t *testing.T) {
	result := scanTree(t, "Movie (2000)", TypeMovie, map[string]string{
		"Movie (2000).mkv": "",
		"Movie (2000).nfo": `<movie><title>Unclosed`,
	})

	if len(result.MediaFiles) != 1 {
		t.Fatalf("len(MediaFiles) = %d, want 1", len(result.MediaFiles))
	}
	feature := result.MediaFiles[0]
	if feature.NFO == nil || feature.NFO.ParseError == "" {
		t.Fatalf("expected a recorded ParseError, got %+v", feature.NFO)
	}
	if len(result.Directory.Warnings) == 0 {
		t.Error("expected a warning about the unreadable sidecar")
	}
}

// TestScanUnresolvedEpisodeNumbering covers absolute-numbered files, the anime
// convention: still a media file, with the unresolved numbering flagged.
func TestScanUnresolvedEpisodeNumbering(t *testing.T) {
	result := scanTree(t, "Some Anime (2015)", TypeTV, map[string]string{
		"Some Anime - 052.mkv": "",
	})

	if len(result.MediaFiles) != 1 {
		t.Fatalf("len(MediaFiles) = %d, want 1; got %s", len(result.MediaFiles), mediaFileNames(result))
	}
	if numbers := result.MediaFiles[0].Video.EpisodeNumbers; numbers != nil {
		t.Errorf("EpisodeNumbers = %v, want nil", numbers)
	}
	if len(result.Directory.Warnings) == 0 {
		t.Error("expected a warning that the episode number could not be resolved")
	}
}

// TestScanEpisodeWithoutNumberingInSeasonFolder covers inheriting the season
// from the folder when the filename alone is not enough.
func TestScanEpisodeWithoutNumberingInSeasonFolder(t *testing.T) {
	result := scanTree(t, "Show (2010)", TypeTV, map[string]string{
		"Season 03/Some Episode Name.mkv": "",
	})

	episode := mediaFileByName(t, result, "Some Episode Name.mkv")
	if episode.Video.SeasonNumber == nil || *episode.Video.SeasonNumber != 3 {
		t.Errorf("SeasonNumber = %v, want 3", derefSeason(episode.Video.SeasonNumber))
	}
	if len(result.Directory.Warnings) == 0 {
		t.Error("expected a warning about the unresolved episode number")
	}
}

func TestScanAbbreviatedSeasonFolderWarns(t *testing.T) {
	result := scanTree(t, "Show (2010)", TypeTV, map[string]string{
		"S01/Show S01E01.mkv": "",
	})

	if len(result.MediaFiles) != 1 {
		t.Fatalf("len(MediaFiles) = %d, want 1", len(result.MediaFiles))
	}
	joined := strings.Join(result.Directory.Warnings, " | ")
	if !strings.Contains(joined, "abbreviated") {
		t.Errorf("expected a warning about the abbreviated season folder, got %q", joined)
	}
}

func TestScanSeasonScopedArtwork(t *testing.T) {
	result := scanTree(t, "Show (2010)", TypeTV, map[string]string{
		"Season 01/Show S01E01.mkv":  "",
		"Season 01/poster.jpg":       "",
		"poster.jpg":                 "",
		"season-specials-poster.jpg": "",
	})

	var seasonScoped, directoryScoped, specials int
	for _, file := range result.Directory.Files {
		if file.Image == nil {
			continue
		}
		switch {
		case file.Image.Scope == ScopeSeason && file.Image.SeasonNumber != nil && *file.Image.SeasonNumber == 0:
			specials++
		case file.Image.Scope == ScopeSeason:
			seasonScoped++
		case file.Image.Scope == ScopeDirectory:
			directoryScoped++
		}
	}
	if seasonScoped != 1 || directoryScoped != 1 || specials != 1 {
		t.Errorf("artwork scoping: season=%d directory=%d specials=%d; files=%s",
			seasonScoped, directoryScoped, specials, directoryFileNames(result))
	}
}

// TestScanDottedNamesMatchSidecars covers why sidecar matching is longest-prefix
// rather than a split on ".": scene-named files are full of dots.
func TestScanDottedNamesMatchSidecars(t *testing.T) {
	result := scanTree(t, "Movie.Name.2019", TypeMovie, map[string]string{
		"Movie.Name.2019.1080p.mkv":           "",
		"Movie.Name.2019.1080p.en.forced.srt": "",
		"Movie.Name.2019.1080p.fr.srt":        "",
		"Movie.Name.2019.1080p.nfo":           `<movie><title>Movie Name</title></movie>`,
	})

	if len(result.MediaFiles) != 1 {
		t.Fatalf("len(MediaFiles) = %d, want 1; got %s", len(result.MediaFiles), mediaFileNames(result))
	}
	feature := result.MediaFiles[0]
	if len(feature.Subtitles) != 2 {
		t.Fatalf("Subtitles = %+v", feature.Subtitles)
	}
	if feature.NFO == nil || feature.NFO.Movie == nil {
		t.Errorf("NFO = %+v", feature.NFO)
	}

	byLanguage := map[string]SubtitleAttributes{}
	for _, subtitle := range feature.Subtitles {
		byLanguage[subtitle.Subtitle.Language] = *subtitle.Subtitle
	}
	if english, ok := byLanguage["en"]; !ok || !english.Forced {
		t.Errorf("english subtitle = %+v", english)
	}
	if french, ok := byLanguage["fr"]; !ok || french.Forced {
		t.Errorf("french subtitle = %+v", french)
	}
}

func TestScanOrphanSubtitleStaysOnDirectory(t *testing.T) {
	result := scanTree(t, "Movie (2000)", TypeMovie, map[string]string{
		"Movie (2000).mkv":           "",
		"Something Unrelated.en.srt": "",
	})

	orphan := directoryFileByName(t, result, "Something Unrelated.en.srt")
	if orphan.Role != RoleSubtitle {
		t.Errorf("role = %q, want %q", orphan.Role, RoleSubtitle)
	}
	if len(result.MediaFiles[0].Subtitles) != 0 {
		t.Errorf("orphan subtitle was wrongly attached: %+v", result.MediaFiles[0].Subtitles)
	}
}

func TestScanRecordsPathsAndMetadata(t *testing.T) {
	result := scanTree(t, "Movie (2000)", TypeMovie, map[string]string{
		"Movie (2000).mkv": "some bytes",
	})

	directory := result.Directory
	if directory.RecordType != RecordTypeDirectory {
		t.Errorf("RecordType = %q", directory.RecordType)
	}
	if directory.FolderName != "Movie (2000)" {
		t.Errorf("FolderName = %q", directory.FolderName)
	}
	if directory.Title != "Movie" || directory.Year != 2000 {
		t.Errorf("Title/Year = %q/%d", directory.Title, directory.Year)
	}
	if !filepath.IsAbs(directory.Path) {
		t.Errorf("Path = %q, want absolute", directory.Path)
	}
	if directory.ScannedAt.IsZero() {
		t.Error("ScannedAt not set")
	}
	// IDs are assigned by MongoDB on insert, never by the scanner.
	if !directory.ID.IsZero() {
		t.Errorf("directory ID = %v, want zero so Mongo can assign it", directory.ID)
	}

	mediaFile := result.MediaFiles[0]
	if !mediaFile.ID.IsZero() || !mediaFile.DirectoryID.IsZero() {
		t.Errorf("media file ids should be zero before storage: %v / %v", mediaFile.ID, mediaFile.DirectoryID)
	}
	if mediaFile.DirectoryPath != directory.Path {
		t.Errorf("DirectoryPath = %q, want %q", mediaFile.DirectoryPath, directory.Path)
	}
	if mediaFile.SizeBytes != int64(len("some bytes")) {
		t.Errorf("SizeBytes = %d", mediaFile.SizeBytes)
	}
	if mediaFile.Extension != "mkv" {
		t.Errorf("Extension = %q", mediaFile.Extension)
	}
	if mediaFile.RelativePath != "Movie (2000).mkv" {
		t.Errorf("RelativePath = %q", mediaFile.RelativePath)
	}
	if mediaFile.ModifiedAt.IsZero() {
		t.Error("ModifiedAt not set")
	}
}

func TestScanEmptyMovieDirectoryWarns(t *testing.T) {
	result := scanTree(t, "Empty Movie (2000)", TypeMovie, map[string]string{
		"poster.jpg": "",
	})

	if len(result.MediaFiles) != 0 {
		t.Fatalf("len(MediaFiles) = %d, want 0", len(result.MediaFiles))
	}
	if len(result.Directory.Warnings) == 0 {
		t.Error("expected a warning that no movie file was found")
	}
}

// TestScanExtrasDoNotWarnAboutNumbering guards against warning noise. Extras
// legitimately have no episode number, so warning about every trailer and
// featurette would bury the genuine unresolved-numbering cases.
func TestScanExtrasDoNotWarnAboutNumbering(t *testing.T) {
	result := scanTree(t, "Show (2010)", TypeTV, map[string]string{
		"Season 01/Show S01E01.mkv":       "",
		"Trailers/Season 1 Teaser.mkv":    "",
		"Behind The Scenes/Making Of.mkv": "",
		"Featurettes/Cast Interviews.mkv": "",
		"Show (2010)-trailer.mkv":         "",
	})

	if len(result.MediaFiles) != 1 {
		t.Fatalf("len(MediaFiles) = %d, want 1; got %s", len(result.MediaFiles), mediaFileNames(result))
	}
	for _, warning := range result.Directory.Warnings {
		if strings.Contains(warning, "numbering") || strings.Contains(warning, "episode number") {
			t.Errorf("extras produced a numbering warning: %q", warning)
		}
	}
}

// TestScanDiscRipWarningExplainsItself covers the wording of the no-media-files
// warning for a ripped disc: the content is present as disc structure, so
// "no movie file found" would send someone hunting a problem that isn't one.
func TestScanDiscRipWarningExplainsItself(t *testing.T) {
	result := scanTree(t, "Old Movie (1975)", TypeMovie, map[string]string{
		"VIDEO_TS/VIDEO_TS.IFO": "",
		"VIDEO_TS/VTS_01_1.VOB": "",
	})

	joined := strings.Join(result.Directory.Warnings, " | ")
	if !strings.Contains(joined, "ripped disc") {
		t.Errorf("warning should explain the disc rip, got %q", joined)
	}
}

func TestScanEmptySeriesWarningNamesEpisodes(t *testing.T) {
	result := scanTree(t, "Show (2010)", TypeTV, map[string]string{
		"poster.jpg": "",
	})

	joined := strings.Join(result.Directory.Warnings, " | ")
	if !strings.Contains(joined, "episode files") {
		t.Errorf("warning should name episodes for a tv directory, got %q", joined)
	}
}

func TestScanErrors(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "missing"), TypeMovie); err == nil {
		t.Error("Scan() succeeded for a missing directory")
	}

	root := buildTree(t, map[string]string{"file.txt": ""})
	if _, err := Scan(filepath.Join(root, "file.txt"), TypeMovie); err == nil {
		t.Error("Scan() succeeded for a path that is not a directory")
	}
}

// TestScanEmptyCollectionsAreNotNil keeps the stored documents predictable:
// a caller reading external_links or files should get an array, not null.
func TestScanEmptyCollectionsAreNotNil(t *testing.T) {
	result := scanTree(t, "Movie (2000)", TypeMovie, map[string]string{
		"Movie (2000).mkv": "",
	})

	if result.Directory.ExternalLinks == nil {
		t.Error("ExternalLinks is nil, want an empty slice")
	}
	if result.Directory.Files == nil {
		t.Error("Files is nil, want an empty slice")
	}
}
