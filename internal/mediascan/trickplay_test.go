package mediascan

import (
	"strconv"
	"strings"
	"testing"
)

// tileFixtures builds the contents of one trickplay resolution folder, keyed by
// path relative to the item directory. Jellyfin names the sheets 0.jpg, 1.jpg
// and so on, and writes nothing else in there.
func tileFixtures(t *testing.T, trickplayFolder, resolutionFolder string, count int) map[string]string {
	t.Helper()

	files := make(map[string]string, count)
	for index := range count {
		name := strconv.Itoa(index) + ".jpg"
		files[trickplayFolder+"/"+resolutionFolder+"/"+name] = jpegFixture(t)
	}
	return files
}

// assertTile checks one tile against the resolution folder it came out of.
func assertTile(t *testing.T, sidecar SidecarFile, wantWidth, wantTileWidth, wantTileHeight, wantIndex int) {
	t.Helper()

	if sidecar.Type != SidecarTrickplay {
		t.Errorf("%s: Type = %q, want %q", sidecar.FileName, sidecar.Type, SidecarTrickplay)
	}
	if sidecar.Category != SidecarCategoryTrickplay {
		t.Errorf("%s: Category = %q, want %q", sidecar.FileName, sidecar.Category, SidecarCategoryTrickplay)
	}

	if sidecar.Trickplay == nil {
		t.Fatalf("%s: Trickplay = nil, want a trickplay record", sidecar.FileName)
	}
	if sidecar.Trickplay.Width != wantWidth {
		t.Errorf("%s: Trickplay.Width = %d, want %d", sidecar.FileName, sidecar.Trickplay.Width, wantWidth)
	}
	if sidecar.Trickplay.TileWidth != wantTileWidth || sidecar.Trickplay.TileHeight != wantTileHeight {
		t.Errorf("%s: tile grid = %dx%d, want %dx%d", sidecar.FileName,
			sidecar.Trickplay.TileWidth, sidecar.Trickplay.TileHeight, wantTileWidth, wantTileHeight)
	}
	if sidecar.Trickplay.TileIndex == nil {
		t.Fatalf("%s: Trickplay.TileIndex = nil, want %d", sidecar.FileName, wantIndex)
	}
	if *sidecar.Trickplay.TileIndex != wantIndex {
		t.Errorf("%s: Trickplay.TileIndex = %d, want %d", sidecar.FileName, *sidecar.Trickplay.TileIndex, wantIndex)
	}
}

// TestTrickplayTilesAttachToTheirMediaFile is the core assertion: tiles two
// folders below the video are recorded on the video's own record, which the
// ordinary prefix match could never do.
func TestTrickplayTilesAttachToTheirMediaFile(t *testing.T) {
	files := map[string]string{
		"The Movie (2019).mkv": "video",
		"poster.jpg":           jpegFixture(t),
	}
	for path, content := range tileFixtures(t, "The Movie (2019).trickplay", "320 - 10x10", 3) {
		files[path] = content
	}

	result := scanTree(t, "The Movie (2019)", TypeMovie, files)
	mediaFile := mediaFileByName(t, result, "The Movie (2019).mkv")

	assertTile(t, mediaSidecarByName(t, mediaFile, "0.jpg"), 320, 10, 10, 0)
	assertTile(t, mediaSidecarByName(t, mediaFile, "1.jpg"), 320, 10, 10, 1)
	assertTile(t, mediaSidecarByName(t, mediaFile, "2.jpg"), 320, 10, 10, 2)

	// The tiles are .jpg, so the image reader still describes each sheet.
	tile := mediaSidecarByName(t, mediaFile, "0.jpg")
	if tile.Image == nil || tile.Image.Codec != "jpeg" {
		t.Errorf("tile Image = %+v, want a jpeg record", tile.Image)
	}
	if tile.RelativePath != "The Movie (2019).trickplay/320 - 10x10/0.jpg" {
		t.Errorf("tile RelativePath = %q, want the path inside the trickplay folder", tile.RelativePath)
	}
}

// TestTrickplayFolderReplacesTheVideoExtension pins the naming rule taken from
// Jellyfin's PathManager: the folder is the video's file name with its
// extension changed, not with ".trickplay" appended. A release-tagged name is
// the case where the two readings visibly differ.
func TestTrickplayFolderReplacesTheVideoExtension(t *testing.T) {
	files := map[string]string{
		"The Movie (2019) [Remux-2160p].mkv": "video",
	}
	for path, content := range tileFixtures(t, "The Movie (2019) [Remux-2160p].trickplay", "320 - 10x10", 1) {
		files[path] = content
	}

	result := scanTree(t, "The Movie (2019)", TypeMovie, files)
	mediaFile := mediaFileByName(t, result, "The Movie (2019) [Remux-2160p].mkv")

	assertTile(t, mediaSidecarByName(t, mediaFile, "0.jpg"), 320, 10, 10, 0)
}

// TestTrickplayRecordsEveryResolution covers a library where more than one
// width was generated, each in its own subfolder with its own tile grid.
func TestTrickplayRecordsEveryResolution(t *testing.T) {
	files := map[string]string{"The Movie (2019).mkv": "video"}
	for path, content := range tileFixtures(t, "The Movie (2019).trickplay", "320 - 10x10", 2) {
		files[path] = content
	}
	files["The Movie (2019).trickplay/640 - 5x5/0.jpg"] = jpegFixture(t)

	result := scanTree(t, "The Movie (2019)", TypeMovie, files)
	mediaFile := mediaFileByName(t, result, "The Movie (2019).mkv")

	byResolution := map[int]int{}
	for _, sidecar := range mediaFile.Sidecars {
		if sidecar.Trickplay != nil {
			byResolution[sidecar.Trickplay.Width]++
		}
	}
	if byResolution[320] != 2 {
		t.Errorf("tiles at width 320 = %d, want 2", byResolution[320])
	}
	if byResolution[640] != 1 {
		t.Errorf("tiles at width 640 = %d, want 1", byResolution[640])
	}

	for _, sidecar := range mediaFile.Sidecars {
		if sidecar.Trickplay != nil && sidecar.Trickplay.Width == 640 {
			assertTile(t, sidecar, 640, 5, 5, 0)
		}
	}
}

// TestTrickplayInSeasonFolder covers the TV layout, where the previews sit
// beside an episode inside its season folder.
func TestTrickplayInSeasonFolder(t *testing.T) {
	files := map[string]string{"Season 01/The Show S01E01.mkv": "episode"}
	for path, content := range tileFixtures(t, "Season 01/The Show S01E01.trickplay", "320 - 10x10", 1) {
		files[path] = content
	}

	result := scanTree(t, "The Show", TypeTV, files)
	mediaFile := mediaFileByName(t, result, "The Show S01E01.mkv")

	assertTile(t, mediaSidecarByName(t, mediaFile, "0.jpg"), 320, 10, 10, 0)
}

// TestTrickplayOrphanFolderWarnsOnce covers what an upgraded media file leaves
// behind: previews naming a video that is no longer there. They land on the
// directory, and the scan says so once rather than once per tile.
func TestTrickplayOrphanFolderWarnsOnce(t *testing.T) {
	files := map[string]string{"The Movie (2019).mkv": "video"}
	for path, content := range tileFixtures(t, "The Movie (2019) [1080p].trickplay", "320 - 10x10", 4) {
		files[path] = content
	}

	result := scanTree(t, "The Movie (2019)", TypeMovie, files)

	mediaFile := mediaFileByName(t, result, "The Movie (2019).mkv")
	if len(mediaFile.Sidecars) != 0 {
		t.Errorf("orphaned tiles landed on the media file: %s", sidecarNames(mediaFile.Sidecars))
	}

	orphaned := 0
	for _, sidecar := range result.Directory.Sidecars {
		if sidecar.Type == SidecarTrickplay {
			orphaned++
		}
	}
	if orphaned != 4 {
		t.Errorf("orphaned tiles on the directory = %d, want 4", orphaned)
	}

	warnings := 0
	for _, warning := range result.Directory.Warnings {
		if strings.Contains(warning, "trickplay") {
			warnings++
		}
	}
	if warnings != 1 {
		t.Errorf("trickplay warnings = %d, want exactly 1; got %v", warnings, result.Directory.Warnings)
	}
}

// TestTrickplayTilesAreNotImages is the reason trickplay is its own category:
// a query for the artwork must not have to wade through hundreds of tiles.
func TestTrickplayTilesAreNotImages(t *testing.T) {
	files := map[string]string{
		"The Movie (2019).mkv":       "video",
		"The Movie (2019)-thumb.jpg": jpegFixture(t),
	}
	for path, content := range tileFixtures(t, "The Movie (2019).trickplay", "320 - 10x10", 5) {
		files[path] = content
	}

	result := scanTree(t, "The Movie (2019)", TypeMovie, files)
	mediaFile := mediaFileByName(t, result, "The Movie (2019).mkv")

	images := sidecarsInCategory(mediaFile.Sidecars, SidecarCategoryImage)
	if len(images) != 1 || images[0].FileName != "The Movie (2019)-thumb.jpg" {
		t.Errorf("image-category sidecars = %s, want only the thumb", sidecarNames(images))
	}
	if got := len(sidecarsInCategory(mediaFile.Sidecars, SidecarCategoryTrickplay)); got != 5 {
		t.Errorf("trickplay-category sidecars = %d, want 5", got)
	}
}

// TestTrickplayFolderIsWalkedButHiddenFoldersAreNot pins the change to
// isIgnoredFolder: trickplay folders are no longer skipped, and everything else
// that was skipped still is.
func TestTrickplayFolderIsWalkedButHiddenFoldersAreNot(t *testing.T) {
	files := map[string]string{
		"The Movie (2019).mkv":                         "video",
		".actors/someone.jpg":                          jpegFixture(t),
		"extrafanart/backdrop1.jpg":                    jpegFixture(t),
		"The Movie (2019).trickplay/320 - 10x10/0.jpg": jpegFixture(t),
	}

	result := scanTree(t, "The Movie (2019)", TypeMovie, files)
	mediaFile := mediaFileByName(t, result, "The Movie (2019).mkv")

	if _, err := findSidecar(mediaFile.Sidecars, "0.jpg"); err != nil {
		t.Errorf("trickplay tile was not recorded: %v", err)
	}

	for _, sidecar := range append(append([]SidecarFile{}, result.Directory.Sidecars...), mediaFile.Sidecars...) {
		if strings.HasPrefix(sidecar.RelativePath, ".actors/") || strings.HasPrefix(sidecar.RelativePath, "extrafanart/") {
			t.Errorf("ignored folder was walked: %q", sidecar.RelativePath)
		}
	}
}

// TestTrickplayLooseFileHasNoResolution covers a file sitting directly in the
// trickplay folder rather than in a resolution subfolder. Jellyfin writes
// nothing there, so it is typed by position but described by nothing.
func TestTrickplayLooseFileHasNoResolution(t *testing.T) {
	result := scanTree(t, "The Movie (2019)", TypeMovie, map[string]string{
		"The Movie (2019).mkv":                 "video",
		"The Movie (2019).trickplay/stray.jpg": jpegFixture(t),
	})

	mediaFile := mediaFileByName(t, result, "The Movie (2019).mkv")
	sidecar := mediaSidecarByName(t, mediaFile, "stray.jpg")

	if sidecar.Type != SidecarTrickplay {
		t.Errorf("Type = %q, want %q from its position", sidecar.Type, SidecarTrickplay)
	}
	if sidecar.Trickplay != nil {
		t.Errorf("Trickplay = %+v, want nil for a file at no resolution", *sidecar.Trickplay)
	}
}

// findSidecar looks a sidecar up by name without failing the test, for the
// cases that assert on absence.
func findSidecar(sidecars []SidecarFile, fileName string) (SidecarFile, error) {
	for _, sidecar := range sidecars {
		if sidecar.FileName == fileName {
			return sidecar, nil
		}
	}
	return SidecarFile{}, errSidecarNotFound
}

var errSidecarNotFound = errNotFound("sidecar not found")

type errNotFound string

func (e errNotFound) Error() string { return string(e) }
