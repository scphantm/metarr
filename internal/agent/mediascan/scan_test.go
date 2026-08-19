package mediascan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/metadata"
	"Metarr/internal/shared/scanmodel"
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
func scanTree(t *testing.T, folderName string, directoryType scanmodel.DirectoryType, files map[string]string) *scanmodel.ScanResult {
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
func mediaFileByName(t *testing.T, result *scanmodel.ScanResult, fileName string) scanmodel.MediaFile {
	t.Helper()
	for _, mediaFile := range result.MediaFiles {
		if mediaFile.FileName == fileName {
			return mediaFile
		}
	}
	t.Fatalf("no media file record named %q; got %s", fileName, mediaFileNames(result))
	return scanmodel.MediaFile{}
}

func mediaFileNames(result *scanmodel.ScanResult) string {
	names := make([]string, 0, len(result.MediaFiles))
	for _, mediaFile := range result.MediaFiles {
		names = append(names, mediaFile.FileName)
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// directorySidecarByName finds a sidecar on the directory record itself,
// excluding the ones filed under a season.
func directorySidecarByName(t *testing.T, result *scanmodel.ScanResult, fileName string) scanmodel.SidecarFile {
	t.Helper()
	for _, sidecar := range result.Directory.Sidecars {
		if sidecar.FileName == fileName {
			return sidecar
		}
	}
	t.Fatalf("no directory sidecar named %q; got %s", fileName, sidecarNames(result.Directory.Sidecars))
	return scanmodel.SidecarFile{}
}

// seasonSidecarByName finds a sidecar filed under one season.
func seasonSidecarByName(t *testing.T, result *scanmodel.ScanResult, seasonNumber int, fileName string) scanmodel.SidecarFile {
	t.Helper()
	season := seasonByNumber(t, result, seasonNumber)
	for _, sidecar := range season.Sidecars {
		if sidecar.FileName == fileName {
			return sidecar
		}
	}
	t.Fatalf("no sidecar named %q on season %d; got %s", fileName, seasonNumber, sidecarNames(season.Sidecars))
	return scanmodel.SidecarFile{}
}

func seasonByNumber(t *testing.T, result *scanmodel.ScanResult, seasonNumber int) scanmodel.TVSeason {
	t.Helper()
	for _, season := range result.Directory.Seasons {
		if season.SeasonNumber == seasonNumber {
			return season
		}
	}
	t.Fatalf("no season record numbered %d; got %+v", seasonNumber, result.Directory.Seasons)
	return scanmodel.TVSeason{}
}

// mediaSidecarByName finds a sidecar attached to one media file record.
func mediaSidecarByName(t *testing.T, mediaFile scanmodel.MediaFile, fileName string) scanmodel.SidecarFile {
	t.Helper()
	for _, sidecar := range mediaFile.Sidecars {
		if sidecar.FileName == fileName {
			return sidecar
		}
	}
	t.Fatalf("no sidecar named %q on %q; got %s", fileName, mediaFile.FileName, sidecarNames(mediaFile.Sidecars))
	return scanmodel.SidecarFile{}
}

// sidecarsInCategory filters a sidecar list down to one category, which is the
// query the categories exist to serve.
func sidecarsInCategory(sidecars []scanmodel.SidecarFile, category scanmodel.SidecarCategory) []scanmodel.SidecarFile {
	matching := make([]scanmodel.SidecarFile, 0, len(sidecars))
	for _, sidecar := range sidecars {
		if sidecar.Category == category {
			matching = append(matching, sidecar)
		}
	}
	return matching
}

func sidecarNames(sidecars []scanmodel.SidecarFile) string {
	names := make([]string, 0, len(sidecars))
	for _, sidecar := range sidecars {
		names = append(names, sidecar.FileName+"("+string(sidecar.Type)+")")
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// TestScanMovieRecordSplit is the core assertion of the storage model: the
// feature is a media file record, and everything else stays on the directory.
func TestScanMovieRecordSplit(t *testing.T) {
	result := scanTree(t, "The Matrix (1999)", scanmodel.TypeMovie, map[string]string{
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
	if feature.RecordType != scanmodel.RecordTypeMediaFile {
		t.Errorf("scanmodel.RecordType = %q", feature.RecordType)
	}
	if feature.Video == nil || feature.Video.Title != "The Matrix" || feature.Video.Year != 1999 {
		t.Errorf("Video = %+v", feature.Video)
	}

	// The feature's own sidecars ride with it: anything whose name begins with
	// the feature's base name belongs to the feature.
	subtitles := sidecarsInCategory(feature.Sidecars, scanmodel.SidecarCategorySubtitle)
	if len(subtitles) != 1 || subtitles[0].FileName != "The Matrix (1999).en.srt" {
		t.Errorf("feature subtitles = %s", sidecarNames(feature.Sidecars))
	}
	if trailer := mediaSidecarByName(t, feature, "The Matrix (1999)-trailer.mkv"); trailer.Type != scanmodel.SidecarTrailer {
		t.Errorf("suffix trailer type = %q, want %q", trailer.Type, scanmodel.SidecarTrailer)
	}
	if feature.Metadata == nil || feature.Metadata.Scope != metadata.ScopeVideo || feature.Metadata.Movie == nil {
		t.Errorf("NFO = %+v", feature.Metadata)
	}
	if feature.Metadata.Title != "The Matrix" {
		t.Errorf("NFO title = %q", feature.Metadata.Title)
	}

	// Artwork, theme music and extras that name no media file stay on the
	// directory, and none of them may appear as a media file.
	for _, name := range []string{"poster.jpg", "fanart.jpg", "backdrop-1.jpg", "theme.mp3", "Making Of.mkv", "Teaser.mkv"} {
		directorySidecarByName(t, result, name)
	}

	wantTypes := map[string]scanmodel.SidecarType{
		"poster.jpg":     scanmodel.SidecarPoster,
		"fanart.jpg":     scanmodel.SidecarFanart,
		"backdrop-1.jpg": scanmodel.SidecarFanart,
		"theme.mp3":      scanmodel.SidecarTheme,
		"Making Of.mkv":  scanmodel.SidecarBehindTheScenes,
		"Teaser.mkv":     scanmodel.SidecarTrailer,
	}
	for name, want := range wantTypes {
		if got := directorySidecarByName(t, result, name).Type; got != want {
			t.Errorf("%s type = %q, want %q", name, got, want)
		}
	}

	// "return all images" is the query the category axis exists for.
	images := sidecarsInCategory(result.Directory.Sidecars, scanmodel.SidecarCategoryImage)
	if len(images) != 3 {
		t.Errorf("directory images = %s, want 3", sidecarNames(images))
	}
}

func TestScanMovieMultipleVersions(t *testing.T) {
	result := scanTree(t, "Blade Runner (1982)", scanmodel.TypeMovie, map[string]string{
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
	result := scanTree(t, "Breaking Bad (2008)", scanmodel.TypeTV, map[string]string{
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
	for _, name := range []string{"tvshow.nfo", "poster.jpg", "theme.mp3", "Season 1 Trailer.mkv"} {
		directorySidecarByName(t, result, name)
	}

	// A file inside a season folder that names no episode belongs to the season.
	if seasonNFO := seasonSidecarByName(t, result, 1, "season.nfo"); seasonNFO.Type != scanmodel.SidecarNFO {
		t.Errorf("season.nfo type = %q, want %q", seasonNFO.Type, scanmodel.SidecarNFO)
	}

	// And so does artwork kept at the series root that names a season, which is
	// where Plex puts it.
	if seasonPoster := seasonSidecarByName(t, result, 1, "Season01-poster.jpg"); seasonPoster.Type != scanmodel.SidecarPoster {
		t.Errorf("Season01-poster.jpg type = %q, want %q", seasonPoster.Type, scanmodel.SidecarPoster)
	}

	// The series' own tvshow.nfo is promoted onto the directory record, but the
	// season.nfo — which shares the same <tvshow> root element — must not leak
	// its own title into it.
	if result.Directory.Metadata == nil || result.Directory.Metadata.Title != "Breaking Bad" {
		t.Errorf("Directory.NFO = %+v, want title %q", result.Directory.Metadata, "Breaking Bad")
	}

	// The episode's own sidecars attach to the episode, not to the directory.
	episode := mediaFileByName(t, result, "Breaking Bad S01E01.mkv")
	if subtitle := mediaSidecarByName(t, episode, "Breaking Bad S01E01.en.srt"); subtitle.Type != scanmodel.SidecarSubtitle {
		t.Errorf("episode subtitle type = %q, want %q", subtitle.Type, scanmodel.SidecarSubtitle)
	}
	if thumb := mediaSidecarByName(t, episode, "Breaking Bad S01E01-thumb.jpg"); thumb.Type != scanmodel.SidecarThumb {
		t.Errorf("episode thumb type = %q, want %q", thumb.Type, scanmodel.SidecarThumb)
	}
	if episode.Metadata == nil || episode.Metadata.Episode == nil || episode.Metadata.Title != "Pilot" {
		t.Errorf("NFO = %+v", episode.Metadata)
	}
	// An episode's own sidecars belong to it and must not also show up on the
	// directory or on its season.
	seasonOne := seasonByNumber(t, result, 1)
	for _, name := range []string{"Breaking Bad S01E01.en.srt", "Breaking Bad S01E01-thumb.jpg", "Breaking Bad S01E01.nfo"} {
		for _, sidecar := range result.Directory.Sidecars {
			if sidecar.FileName == name {
				t.Errorf("%q should belong to its episode record, not the directory", name)
			}
		}
		for _, sidecar := range seasonOne.Sidecars {
			if sidecar.FileName == name {
				t.Errorf("%q should belong to its episode record, not its season", name)
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

	// The season index records the seasons present on disk, ordered by number.
	if result.Directory.Metadata.TVShow == nil {
		t.Fatalf("Directory.Metadata.TVShow = nil, want season index")
	}
	seasons := result.Directory.Metadata.TVShow.Seasons
	wantSeasonNumbers := []int{0, 1, 2}
	if len(seasons) != len(wantSeasonNumbers) {
		t.Fatalf("Seasons = %+v, want numbers %v", seasons, wantSeasonNumbers)
	}
	for i, want := range wantSeasonNumbers {
		if seasons[i].SeasonNumber != want {
			t.Errorf("Seasons[%d].SeasonNumber = %d, want %d", i, seasons[i].SeasonNumber, want)
		}
	}

	// The season records carry the same numbering, plus the folder each was
	// found in.
	if len(result.Directory.Seasons) != len(wantSeasonNumbers) {
		t.Fatalf("Directory.Seasons = %+v, want numbers %v", result.Directory.Seasons, wantSeasonNumbers)
	}
	for i, want := range wantSeasonNumbers {
		if result.Directory.Seasons[i].SeasonNumber != want {
			t.Errorf("Directory.Seasons[%d].SeasonNumber = %d, want %d", i, result.Directory.Seasons[i].SeasonNumber, want)
		}
	}
	if folder := seasonByNumber(t, result, 1).FolderName; folder != "Season 01" {
		t.Errorf("season 1 folder = %q, want %q", folder, "Season 01")
	}
	if folder := seasonByNumber(t, result, 0).FolderName; folder != "Specials" {
		t.Errorf("specials folder = %q, want %q", folder, "Specials")
	}

	// A season whose folder holds nothing but episodes is still a season.
	if sidecars := seasonByNumber(t, result, 2).Sidecars; len(sidecars) != 0 {
		t.Errorf("season 2 sidecars = %s, want none", sidecarNames(sidecars))
	}
}

// TestScanSeriesNFOFirstWins covers the "already initialized" guard: once a
// directory-scoped tvshow-rooted NFO has set Directory.NFO, nothing else can
// overwrite it — not a second directory-level file sharing the same root
// element, and not a season.nfo (WalkDir visits files in lexical order, so
// tvshow.nfo is seen before zzz-duplicate.nfo here).
func TestScanSeriesNFOFirstWins(t *testing.T) {
	result := scanTree(t, "Breaking Bad (2008)", scanmodel.TypeTV, map[string]string{
		"tvshow.nfo":                        `<tvshow><title>Breaking Bad</title></tvshow>`,
		"zzz-duplicate.nfo":                 `<tvshow><title>Duplicate Show</title></tvshow>`,
		"Season 01/season.nfo":              `<tvshow><title>Season 1</title></tvshow>`,
		"Season 01/Breaking Bad S01E01.mkv": "",
	})

	if result.Directory.Metadata == nil || result.Directory.Metadata.Title != "Breaking Bad" {
		t.Fatalf("Directory.NFO = %+v, want the first tvshow.nfo's title", result.Directory.Metadata)
	}
}

// TestScanSeriesLinkSeparation covers the identity split: series ids on the
// directory, episode ids on the episode.
func TestScanSeriesLinkSeparation(t *testing.T) {
	result := scanTree(t, "Breaking Bad (2008)", scanmodel.TypeTV, map[string]string{
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

	if result.Directory.Metadata == nil {
		t.Fatal("Directory.Metadata = nil, want the series metadata carrying its links")
	}
	links := result.Directory.Metadata.ExternalLinks
	if !hasLink(links, metadata.Link{Key: "tvdb", Value: "81189"}) {
		t.Errorf("directory external_links missing the series tvdb id: %+v", links)
	}
	if !hasLink(links, metadata.Link{Key: "imdb", Value: "tt0903747"}) {
		t.Errorf("directory external_links missing the imdb id: %+v", links)
	}
	if !hasLink(links, metadata.Link{Key: "youtube", Value: "HhesaQXLuRY"}) {
		t.Errorf("directory external_links missing the youtube trailer id: %+v", links)
	}

	// Episode ids are deliberately kept out of the directory's links.
	for _, episodeID := range []string{"349232", "349233"} {
		if hasLink(links, metadata.Link{Key: "tvdb", Value: episodeID}) {
			t.Errorf("episode id %s leaked into the directory's external_links: %+v", episodeID, links)
		}
	}

	firstLinks := mediaFileLinks(t, mediaFileByName(t, result, "Breaking Bad S01E01.mkv"))
	if !hasLink(firstLinks, metadata.Link{Key: "tvdb", Value: "349232"}) {
		t.Errorf("episode 1 external_links = %+v", firstLinks)
	}
	secondLinks := mediaFileLinks(t, mediaFileByName(t, result, "Breaking Bad S01E02.mkv"))
	if !hasLink(secondLinks, metadata.Link{Key: "tvdb", Value: "349233"}) {
		t.Errorf("episode 2 external_links = %+v", secondLinks)
	}
	if hasLink(firstLinks, metadata.Link{Key: "tvdb", Value: "349233"}) {
		t.Error("episode ids bled between episodes")
	}
}

// mediaFileLinks reads a media file's provider ids, which live on its own
// metadata record rather than beside it.
func mediaFileLinks(t *testing.T, mediaFile scanmodel.MediaFile) []metadata.Link {
	t.Helper()
	if mediaFile.Metadata == nil {
		t.Fatalf("%q has no metadata record, so no external links", mediaFile.FileName)
	}
	return mediaFile.Metadata.ExternalLinks
}

func hasLink(links []metadata.Link, want metadata.Link) bool {
	for _, link := range links {
		if link == want {
			return true
		}
	}
	return false
}

// TestScanLegacyMultiEpisodeNFO covers a v21-era sidecar holding several
// episodes. The NFO reader accepts the concatenated layout; the metadata model
// represents the file by its first episode.
func TestScanLegacyMultiEpisodeNFO(t *testing.T) {
	result := scanTree(t, "Show (2010)", scanmodel.TypeTV, map[string]string{
		"Season 01/Show S01E01-E02.mkv": "",
		"Season 01/Show S01E01-E02.nfo": `<episodedetails><title>Part 1</title><episode>1</episode>` +
			`<uniqueid type="tvdb">1</uniqueid></episodedetails>` +
			`<episodedetails><title>Part 2</title><episode>2</episode>` +
			`<uniqueid type="tvdb">2</uniqueid></episodedetails>`,
	})

	episode := mediaFileByName(t, result, "Show S01E01-E02.mkv")
	// Both episode numbers still come from the file name.
	if got := episode.Video.EpisodeNumbers; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("EpisodeNumbers = %v, want [1 2]", got)
	}
	// The sidecar is represented by its first episode.
	if episode.Metadata == nil || episode.Metadata.Episode == nil || episode.Metadata.Title != "Part 1" {
		t.Fatalf("NFO = %+v", episode.Metadata)
	}
	if links := mediaFileLinks(t, episode); !hasLink(links, metadata.Link{Key: "tvdb", Value: "1"}) {
		t.Errorf("external_links missing tvdb=1: %+v", links)
	}
}

// TestScanDiscStructure covers a ripped DVD: one item, not one media file per
// VOB.
func TestScanDiscStructure(t *testing.T) {
	result := scanTree(t, "Old Movie (1975)", scanmodel.TypeMovie, map[string]string{
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
		sidecar := directorySidecarByName(t, result, name)
		if sidecar.Type != scanmodel.SidecarDiscStructure || sidecar.Category != scanmodel.SidecarCategoryDiscStructure {
			t.Errorf("%s = %q/%q, want %q/%q", name, sidecar.Type, sidecar.Category, scanmodel.SidecarDiscStructure, scanmodel.SidecarCategoryDiscStructure)
		}
	}
}

func TestScanMusicVideos(t *testing.T) {
	result := scanTree(t, "a-ha", scanmodel.TypeMusicVideo, map[string]string{
		"a-ha - Take On Me.mp4":            "",
		"a-ha - The Sun Always Shines.mp4": "",
		"folder.jpg":                       "",
	})

	if len(result.MediaFiles) != 2 {
		t.Fatalf("len(MediaFiles) = %d, want 2; got %s", len(result.MediaFiles), mediaFileNames(result))
	}

	video := mediaFileByName(t, result, "a-ha - Take On Me.mp4")
	if video.Video.Title != "Take On Me" {
		t.Errorf("Title = %q, want %q", video.Video.Title, "Take On Me")
	}
}

func TestScanMusicVideosNested(t *testing.T) {
	// Jellyfin allows arbitrary nesting under a music video library.
	result := scanTree(t, "Various", scanmodel.TypeMusicVideo, map[string]string{
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
	result := scanTree(t, "Movie (2000)", scanmodel.TypeMovie, map[string]string{
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
	if len(result.Directory.Sidecars) != 0 {
		t.Errorf("junk reached the directory record: %s", sidecarNames(result.Directory.Sidecars))
	}
}

// TestScanCorruptNFO covers the rule that one bad sidecar cannot fail a scan.
func TestScanCorruptNFO(t *testing.T) {
	result := scanTree(t, "Movie (2000)", scanmodel.TypeMovie, map[string]string{
		"Movie (2000).mkv": "",
		"Movie (2000).nfo": `<movie><title>Unclosed`,
	})

	if len(result.MediaFiles) != 1 {
		t.Fatalf("len(MediaFiles) = %d, want 1", len(result.MediaFiles))
	}
	feature := result.MediaFiles[0]
	if feature.Metadata == nil || feature.Metadata.ParseError == "" {
		t.Fatalf("expected a recorded ParseError, got %+v", feature.Metadata)
	}
	if len(result.Directory.Warnings) == 0 {
		t.Error("expected a warning about the unreadable sidecar")
	}
}

// TestScanUnresolvedEpisodeNumbering covers absolute-numbered files, the anime
// convention: still a media file, with the unresolved numbering flagged.
func TestScanUnresolvedEpisodeNumbering(t *testing.T) {
	result := scanTree(t, "Some Anime (2015)", scanmodel.TypeTV, map[string]string{
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
	result := scanTree(t, "Show (2010)", scanmodel.TypeTV, map[string]string{
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
	result := scanTree(t, "Show (2010)", scanmodel.TypeTV, map[string]string{
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

// TestScanSeasonScopedArtwork covers the two ways artwork says which season it
// belongs to: by sitting in the season's folder, or — the layout Plex documents
// — by naming the season while sitting beside the series.
func TestScanSeasonScopedArtwork(t *testing.T) {
	result := scanTree(t, "Show (2010)", scanmodel.TypeTV, map[string]string{
		"Season 01/Show S01E01.mkv":  "",
		"Season 02/Show S02E01.mkv":  "",
		"Season 01/poster.jpg":       "",
		"poster.jpg":                 "",
		"fanart.jpg":                 "",
		"Season01-poster.jpg":        "",
		"Season01.jpg":               "",
		"Season 02-poster.jpg":       "",
		"season02-banner.jpg":        "",
		"Season01-thumb.jpg":         "",
		"season-specials-poster.jpg": "",
	})

	// Artwork inside a season folder belongs to that season.
	if seasonPoster := seasonSidecarByName(t, result, 1, "poster.jpg"); seasonPoster.Type != scanmodel.SidecarPoster {
		t.Errorf("season poster type = %q, want %q", seasonPoster.Type, scanmodel.SidecarPoster)
	}

	// Artwork at the series root that names a season belongs to that season, in
	// every spelling both servers accept.
	wantSeasonArtwork := []struct {
		fileName     string
		seasonNumber int
		wantType     scanmodel.SidecarType
	}{
		{"Season01-poster.jpg", 1, scanmodel.SidecarPoster},
		{"Season01.jpg", 1, scanmodel.SidecarPoster},
		{"Season01-thumb.jpg", 1, scanmodel.SidecarThumb},
		{"Season 02-poster.jpg", 2, scanmodel.SidecarPoster},
		{"season02-banner.jpg", 2, scanmodel.SidecarBanner},
		{"season-specials-poster.jpg", 0, scanmodel.SidecarPoster},
	}
	for _, want := range wantSeasonArtwork {
		got := seasonSidecarByName(t, result, want.seasonNumber, want.fileName)
		if got.Type != want.wantType {
			t.Errorf("%s type = %q, want %q", want.fileName, got.Type, want.wantType)
		}
	}

	// The series' own artwork stays put: naming a season is what moves a file,
	// not merely sitting at the root.
	for _, name := range []string{"poster.jpg", "fanart.jpg"} {
		directorySidecarByName(t, result, name)
	}
	if images := sidecarsInCategory(result.Directory.Sidecars, scanmodel.SidecarCategoryImage); len(images) != 2 {
		t.Errorf("directory images = %s, want just the series' own two", sidecarNames(images))
	}
}

// TestScanSeasonKnownOnlyFromArtwork covers a season with no folder of its own.
// The artwork asserts the season exists, so a record is created for it rather
// than the file being filed under the series and effectively lost.
func TestScanSeasonKnownOnlyFromArtwork(t *testing.T) {
	result := scanTree(t, "Show (2010)", scanmodel.TypeTV, map[string]string{
		"Season 01/Show S01E01.mkv": "",
		"Season03-poster.jpg":       "",
	})

	third := seasonByNumber(t, result, 3)
	if third.FolderName != "" {
		t.Errorf("season 3 folder = %q, want empty — it has no directory on disk", third.FolderName)
	}
	if len(third.Sidecars) != 1 || third.Sidecars[0].FileName != "Season03-poster.jpg" {
		t.Errorf("season 3 sidecars = %s", sidecarNames(third.Sidecars))
	}

	// And it is indexed alongside the season that does have a folder.
	if result.Directory.Metadata == nil || result.Directory.Metadata.TVShow == nil {
		t.Fatal("Directory.Metadata.TVShow = nil, want the season index")
	}
	if seasons := result.Directory.Metadata.TVShow.Seasons; len(seasons) != 2 {
		t.Errorf("season index = %+v, want seasons 1 and 3", seasons)
	}
}

// TestScanSeasonArtworkNamesOnlyApplyToSeries guards the gate on the rule: a
// movie has no seasons, so a stray "season01.jpg" beside a film must not conjure
// one for it to belong to.
func TestScanSeasonArtworkNamesOnlyApplyToSeries(t *testing.T) {
	result := scanTree(t, "Movie (2000)", scanmodel.TypeMovie, map[string]string{
		"Movie (2000).mkv":    "",
		"Season01-poster.jpg": "",
	})

	if len(result.Directory.Seasons) != 0 {
		t.Errorf("a movie grew seasons: %+v", result.Directory.Seasons)
	}
	directorySidecarByName(t, result, "Season01-poster.jpg")
}

// TestScanDottedNamesMatchSidecars covers why sidecar matching is longest-prefix
// rather than a split on ".": scene-named files are full of dots.
func TestScanDottedNamesMatchSidecars(t *testing.T) {
	result := scanTree(t, "Movie.Name.2019", scanmodel.TypeMovie, map[string]string{
		"Movie.Name.2019.1080p.mkv":           "",
		"Movie.Name.2019.1080p.en.forced.srt": "",
		"Movie.Name.2019.1080p.fr.srt":        "",
		"Movie.Name.2019.1080p.nfo":           `<movie><title>Movie Name</title></movie>`,
	})

	if len(result.MediaFiles) != 1 {
		t.Fatalf("len(MediaFiles) = %d, want 1; got %s", len(result.MediaFiles), mediaFileNames(result))
	}
	feature := result.MediaFiles[0]
	subtitles := sidecarsInCategory(feature.Sidecars, scanmodel.SidecarCategorySubtitle)
	if len(subtitles) != 2 {
		t.Fatalf("subtitles = %s, want 2", sidecarNames(feature.Sidecars))
	}
	if feature.Metadata == nil || feature.Metadata.Movie == nil {
		t.Errorf("NFO = %+v", feature.Metadata)
	}

	for _, name := range []string{"Movie.Name.2019.1080p.en.forced.srt", "Movie.Name.2019.1080p.fr.srt", "Movie.Name.2019.1080p.nfo"} {
		mediaSidecarByName(t, feature, name)
	}
}

func TestScanOrphanSubtitleStaysOnDirectory(t *testing.T) {
	result := scanTree(t, "Movie (2000)", scanmodel.TypeMovie, map[string]string{
		"Movie (2000).mkv":           "",
		"Something Unrelated.en.srt": "",
	})

	orphan := directorySidecarByName(t, result, "Something Unrelated.en.srt")
	if orphan.Type != scanmodel.SidecarSubtitle || orphan.Category != scanmodel.SidecarCategorySubtitle {
		t.Errorf("orphan = %q/%q, want %q/%q", orphan.Type, orphan.Category, scanmodel.SidecarSubtitle, scanmodel.SidecarCategorySubtitle)
	}
	if sidecars := result.MediaFiles[0].Sidecars; len(sidecars) != 0 {
		t.Errorf("orphan subtitle was wrongly attached: %s", sidecarNames(sidecars))
	}
}

func TestScanRecordsPathsAndMetadata(t *testing.T) {
	result := scanTree(t, "Movie (2000)", scanmodel.TypeMovie, map[string]string{
		"Movie (2000).mkv": "some bytes",
	})

	directory := result.Directory
	if directory.RecordType != scanmodel.RecordTypeTVSeries {
		t.Errorf("scanmodel.RecordType = %q", directory.RecordType)
	}
	if directory.FolderName != "Movie (2000)" {
		t.Errorf("FolderName = %q", directory.FolderName)
	}
	if directory.Metadata == nil || directory.Metadata.Title != "Movie" || directory.Metadata.Year != 2000 {
		t.Errorf("Title/Year = %+v", directory.Metadata)
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
	result := scanTree(t, "Empty Movie (2000)", scanmodel.TypeMovie, map[string]string{
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
	result := scanTree(t, "Show (2010)", scanmodel.TypeTV, map[string]string{
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
	result := scanTree(t, "Old Movie (1975)", scanmodel.TypeMovie, map[string]string{
		"VIDEO_TS/VIDEO_TS.IFO": "",
		"VIDEO_TS/VTS_01_1.VOB": "",
	})

	joined := strings.Join(result.Directory.Warnings, " | ")
	if !strings.Contains(joined, "ripped disc") {
		t.Errorf("warning should explain the disc rip, got %q", joined)
	}
}

func TestScanEmptySeriesWarningNamesEpisodes(t *testing.T) {
	result := scanTree(t, "Show (2010)", scanmodel.TypeTV, map[string]string{
		"poster.jpg": "",
	})

	joined := strings.Join(result.Directory.Warnings, " | ")
	if !strings.Contains(joined, "episode files") {
		t.Errorf("warning should name episodes for a tv directory, got %q", joined)
	}
}

func TestScanErrors(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "missing"), scanmodel.TypeMovie); err == nil {
		t.Error("Scan() succeeded for a missing directory")
	}

	root := buildTree(t, map[string]string{"file.txt": ""})
	if _, err := Scan(filepath.Join(root, "file.txt"), scanmodel.TypeMovie); err == nil {
		t.Error("Scan() succeeded for a path that is not a directory")
	}
}

// TestScanEmptyCollectionsAreNotNil keeps the stored documents predictable:
// a caller reading external_links or sidecars should get an array, not null.
func TestScanEmptyCollectionsAreNotNil(t *testing.T) {
	result := scanTree(t, "Movie (2000)", scanmodel.TypeMovie, map[string]string{
		"Movie (2000).mkv": "",
		"Movie (2000).nfo": `<movie><title>Movie</title></movie>`,
	})

	if result.Directory.Metadata == nil {
		t.Fatal("Directory.Metadata is nil")
	}
	if result.Directory.Metadata.ExternalLinks == nil {
		t.Error("Directory.Metadata.ExternalLinks is nil, want an empty slice")
	}
	if result.Directory.Sidecars == nil {
		t.Error("Directory.Sidecars is nil, want an empty slice")
	}
	if result.MediaFiles[0].Sidecars == nil {
		t.Error("scanmodel.MediaFile.Sidecars is nil, want an empty slice")
	}
	// The NFO here carries no provider ids at all, which is exactly the case
	// that must still encode as an array rather than null.
	if result.MediaFiles[0].Metadata == nil {
		t.Fatal("scanmodel.MediaFile.Metadata is nil")
	}
	if result.MediaFiles[0].Metadata.ExternalLinks == nil {
		t.Error("scanmodel.MediaFile.Metadata.ExternalLinks is nil, want an empty slice")
	}
}

// TestScanExtraAttachesToTheFileItNames covers the routing rule that decides
// which of the three owners a sidecar lands on. A trailer named for an episode
// belongs to that episode; one sitting in a Trailers folder names nothing, so it
// belongs to the series.
func TestScanExtraAttachesToTheFileItNames(t *testing.T) {
	result := scanTree(t, "Show (2010)", scanmodel.TypeTV, map[string]string{
		"Season 01/Show S01E01.mkv":         "",
		"Season 01/Show S01E01-trailer.mkv": "",
		"Season 01/season-banner.jpg":       "",
		"Trailers/Teaser.mkv":               "",
	})

	episode := mediaFileByName(t, result, "Show S01E01.mkv")
	if trailer := mediaSidecarByName(t, episode, "Show S01E01-trailer.mkv"); trailer.Type != scanmodel.SidecarTrailer {
		t.Errorf("episode trailer type = %q, want %q", trailer.Type, scanmodel.SidecarTrailer)
	}

	// Named for no media file, but inside a season folder: the season owns it.
	seasonSidecarByName(t, result, 1, "season-banner.jpg")

	// Named for no media file and in no season folder: the directory owns it.
	if teaser := directorySidecarByName(t, result, "Teaser.mkv"); teaser.Type != scanmodel.SidecarTrailer {
		t.Errorf("folder trailer type = %q, want %q", teaser.Type, scanmodel.SidecarTrailer)
	}
}

// TestScanUnrecognizedFileIsRecordedAsUnknown covers the fallback: a file the
// table cannot name is still recorded, because losing it silently would be
// worse than admitting we don't know what it is.
func TestScanUnrecognizedFileIsRecordedAsUnknown(t *testing.T) {
	result := scanTree(t, "Movie (2000)", scanmodel.TypeMovie, map[string]string{
		"Movie (2000).mkv": "",
		"readme.txt":       "",
	})

	unknown := directorySidecarByName(t, result, "readme.txt")
	if unknown.Type != scanmodel.SidecarUnknown || unknown.Category != scanmodel.SidecarCategoryUnknown {
		t.Errorf("readme.txt = %q/%q, want %q/%q", unknown.Type, unknown.Category, scanmodel.SidecarUnknown, scanmodel.SidecarCategoryUnknown)
	}
}

// TestScanRecordsSidecarFileFacts checks the plain file metadata on a sidecar
// record, which is what makes it usable without going back to disk.
func TestScanRecordsSidecarFileFacts(t *testing.T) {
	result := scanTree(t, "Movie (2000)", scanmodel.TypeMovie, map[string]string{
		"Movie (2000).mkv": "",
		"poster.jpg":       "some bytes",
	})

	poster := directorySidecarByName(t, result, "poster.jpg")
	if poster.RelativePath != "poster.jpg" {
		t.Errorf("RelativePath = %q", poster.RelativePath)
	}
	if poster.Extension != "jpg" {
		t.Errorf("Extension = %q, want %q", poster.Extension, "jpg")
	}
	if poster.SizeBytes != int64(len("some bytes")) {
		t.Errorf("SizeBytes = %d, want %d", poster.SizeBytes, len("some bytes"))
	}
	if poster.ModifiedAt.IsZero() {
		t.Error("ModifiedAt not set")
	}
}

// TestScanDisabledTypeIsNotAssignedFromFolderContext covers the path that would
// otherwise make "disabled" a half-truth. A video inside a Trailers folder is
// named a trailer by its position, without the classification table ever being
// consulted — so switching the trailer type off has to be checked on that path
// too, not just on the pattern-matching one.
func TestScanDisabledTypeIsNotAssignedFromFolderContext(t *testing.T) {
	files := map[string]string{
		"Movie (2000).mkv":    "",
		"Trailers/Teaser.mkv": "",
		"poster.jpg":          "",
	}

	// With the built-in table, position alone names it a trailer.
	result := scanTree(t, "Movie (2000)", scanmodel.TypeMovie, files)
	if teaser := directorySidecarByName(t, result, "Teaser.mkv"); teaser.Type != scanmodel.SidecarTrailer {
		t.Fatalf("Teaser.mkv = %q, want %q before the type is disabled", teaser.Type, scanmodel.SidecarTrailer)
	}

	withSidecarRegistry(t, registryWithTrailerDisabled(t))

	result = scanTree(t, "Movie (2000)", scanmodel.TypeMovie, files)
	teaser := directorySidecarByName(t, result, "Teaser.mkv")
	if teaser.Type == scanmodel.SidecarTrailer {
		t.Error("a video in Trailers/ was still classified as a trailer after the type was disabled")
	}
	if teaser.Type != scanmodel.SidecarUnknown || teaser.Category != scanmodel.SidecarCategoryUnknown {
		t.Errorf("Teaser.mkv = %q/%q, want %q/%q", teaser.Type, teaser.Category, scanmodel.SidecarUnknown, scanmodel.SidecarCategoryUnknown)
	}

	// Only the disabled type changed; everything else classifies as before.
	if poster := directorySidecarByName(t, result, "poster.jpg"); poster.Type != scanmodel.SidecarPoster {
		t.Errorf("poster.jpg = %q, want %q — disabling trailer should not touch it", poster.Type, scanmodel.SidecarPoster)
	}
	if len(result.MediaFiles) != 1 {
		t.Errorf("len(MediaFiles) = %d, want 1", len(result.MediaFiles))
	}
}

// registryWithTrailerDisabled returns the built-in table with the trailer entry
// switched off, which is exactly what the ordering endpoint does when it is
// given order 0 for that id.
func registryWithTrailerDisabled(t *testing.T) *scanmodel.SidecarRegistry {
	t.Helper()

	definitions := appconfig.DefaultSidecarTypes()
	disabled := false
	for i := range definitions {
		if definitions[i].Type == string(scanmodel.SidecarTrailer) {
			definitions[i].Order = 0
			disabled = true
		}
	}
	if !disabled {
		t.Fatal("the default table no longer carries a trailer type")
	}

	registry, err := scanmodel.NewSidecarRegistry(definitions)
	if err != nil {
		t.Fatalf("scanmodel.NewSidecarRegistry() error = %v", err)
	}
	return registry
}

// withSidecarRegistry installs registry for the duration of one test. The
// scanner reads the active registry through a package global, so a test that
// needs a different classification table has to swap it and put it back.
func withSidecarRegistry(t *testing.T, registry *scanmodel.SidecarRegistry) {
	t.Helper()
	previous := scanmodel.ActiveSidecarRegistry()
	scanmodel.SetSidecarRegistry(registry)
	t.Cleanup(func() { scanmodel.SetSidecarRegistry(previous) })
}
