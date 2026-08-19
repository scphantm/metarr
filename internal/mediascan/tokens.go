package mediascan

import (
	"regexp"
	"strconv"
	"strings"
)

// This file holds the naming vocabularies defined by the Jellyfin and Plex
// media organization specifications:
//
//	https://jellyfin.org/docs/general/server/media/movies
//	https://jellyfin.org/docs/general/server/media/shows/
//	https://jellyfin.org/docs/general/server/media/music-videos/
//	https://support.plex.tv/articles/naming-and-organizing-your-tv-show-files/
//
// Every lookup here is case-insensitive on a lowercased name, which is also
// what lets Plex's Title Case folder names ("Behind The Scenes") and
// Jellyfin's lowercase ones ("behind the scenes") resolve to the same thing.

// fileClass is the broad category implied by a file's extension, decided before
// any name parsing happens.
type fileClass int

const (
	classOther fileClass = iota
	classVideo
	classSubtitle
	classImage
	classAudio
	classNFO
)

var extensionClasses = map[string]fileClass{
	// Video containers.
	"mkv": classVideo, "mp4": classVideo, "avi": classVideo, "m4v": classVideo,
	"mov": classVideo, "wmv": classVideo, "mpg": classVideo, "mpeg": classVideo,
	"m2ts": classVideo, "mts": classVideo, "ts": classVideo, "vob": classVideo,
	"iso": classVideo, "divx": classVideo, "flv": classVideo, "webm": classVideo,
	"ogv": classVideo, "3gp": classVideo, "m2v": classVideo, "rm": classVideo,
	"rmvb": classVideo, "asf": classVideo,

	// External subtitles.
	"srt": classSubtitle, "ass": classSubtitle, "ssa": classSubtitle,
	"sub": classSubtitle, "idx": classSubtitle, "vtt": classSubtitle,
	"sup": classSubtitle, "smi": classSubtitle, "pgs": classSubtitle,

	// Artwork. .tbn is a Kodi-era poster extension.
	"jpg": classImage, "jpeg": classImage, "png": classImage, "webp": classImage,
	"bmp": classImage, "gif": classImage, "tbn": classImage,

	// Audio, only meaningful here as theme music.
	"mp3": classAudio, "flac": classAudio, "m4a": classAudio, "ogg": classAudio,
	"wav": classAudio, "opus": classAudio,

	"nfo": classNFO,
}

// classifyExtension returns the class for a lowercased extension without its
// leading dot.
func classifyExtension(extension string) fileClass {
	return extensionClasses[extension]
}

// extrasFolderTypes maps Jellyfin's fixed set of extras folder names to the
// extra type recorded for videos found inside them. A video in one of these
// folders is never a media file, no matter what it is named.
var extrasFolderTypes = map[string]string{
	"behind the scenes": ExtraBehindTheScenes,
	"deleted scenes":    ExtraDeletedScene,
	"interviews":        ExtraInterview,
	"scenes":            ExtraScene,
	"samples":           ExtraSample,
	"shorts":            ExtraShort,
	"featurettes":       ExtraFeaturette,
	"clips":             ExtraClip,
	"other":             ExtraOther,
	"extras":            ExtraExtra,
	"trailers":          ExtraTrailer,
	"theme-music":       ExtraThemeMusic,
	"backdrops":         ExtraBackdrop,
}

// Extra type names, recorded on extra videos and used to map folders and
// filename suffixes onto one vocabulary.
const (
	ExtraBehindTheScenes = "behind_the_scenes"
	ExtraDeletedScene    = "deleted_scene"
	ExtraInterview       = "interview"
	ExtraScene           = "scene"
	ExtraSample          = "sample"
	ExtraShort           = "short"
	ExtraFeaturette      = "featurette"
	ExtraClip            = "clip"
	ExtraOther           = "other"
	ExtraExtra           = "extra"
	ExtraTrailer         = "trailer"
	ExtraThemeMusic      = "theme_music"
	ExtraBackdrop        = "backdrop"
)

// extrasFolderType reports the extra type for a folder name, if it is one of
// the recognized extras folders.
func extrasFolderType(folderName string) (string, bool) {
	extraType, ok := extrasFolderTypes[strings.ToLower(folderName)]
	return extraType, ok
}

// extraTypeToSidecarType maps the extras vocabulary onto sidecar types. Most
// values are already the same string, but the mapping is written out because
// the two vocabularies are not the same size: the extras vocabulary
// distinguishes samples, clips and generic extras, which all classify as
// other_extra, and it covers theme music and backdrops, which are not extra
// videos at all.
var extraTypeToSidecarType = map[string]SidecarType{
	ExtraTrailer:         SidecarTrailer,
	ExtraBehindTheScenes: SidecarBehindTheScenes,
	ExtraDeletedScene:    SidecarDeletedScene,
	ExtraInterview:       SidecarInterview,
	ExtraScene:           SidecarScene,
	ExtraShort:           SidecarShort,
	ExtraFeaturette:      SidecarFeaturette,
	ExtraSample:          SidecarOtherExtra,
	ExtraClip:            SidecarOtherExtra,
	ExtraOther:           SidecarOtherExtra,
	ExtraExtra:           SidecarOtherExtra,
	ExtraThemeMusic:      SidecarTheme,
	ExtraBackdrop:        SidecarFanart,
}

// extrasSuffixTypes maps the filename suffix stems Jellyfin recognizes to the
// same extra type vocabulary.
var extrasSuffixTypes = map[string]string{
	"trailer":         ExtraTrailer,
	"sample":          ExtraSample,
	"scene":           ExtraScene,
	"clip":            ExtraClip,
	"interview":       ExtraInterview,
	"behindthescenes": ExtraBehindTheScenes,
	"deleted":         ExtraDeletedScene,
	"deletedscene":    ExtraDeletedScene,
	"featurette":      ExtraFeaturette,
	"short":           ExtraShort,
	"other":           ExtraOther,
	"extra":           ExtraExtra,
}

// spaceSeparableExtrasSuffixes are the only stems allowed to be introduced by a
// space. Jellyfin documents " trailer" and " sample", and restricting the rest
// to -, . and _ matters: a movie legitimately titled "The Other" or "The Short"
// would otherwise be misfiled as an extra rather than as the feature.
var spaceSeparableExtrasSuffixes = map[string]bool{
	"trailer": true,
	"sample":  true,
}

// discStructureFolders hold the internals of a ripped DVD or Blu-ray. Every
// file beneath one describes a single playable item, so they are recorded as
// disc structure rather than turning one rip into dozens of media files.
var discStructureFolders = map[string]bool{
	"video_ts":    true,
	"audio_ts":    true,
	"bdmv":        true,
	"certificate": true,
	"stream":      true,
	"playlist":    true,
	"clipinf":     true,
	"backup":      true,
}

func isDiscStructureFolder(folderName string) bool {
	return discStructureFolders[strings.ToLower(folderName)]
}

// trickplayFolderSuffix ends the folder Jellyfin writes its scrubbing previews
// into when it is configured to keep them beside the media. The video's
// extension is replaced rather than appended, so "Movie (2019).mkv" is
// previewed by "Movie (2019).trickplay".
const trickplayFolderSuffix = ".trickplay"

// trickplayFolderBaseName reports whether a folder holds trickplay previews,
// returning the base name of the media file they belong to.
func trickplayFolderBaseName(folderName string) (string, bool) {
	if len(folderName) <= len(trickplayFolderSuffix) {
		return "", false
	}
	if !strings.HasSuffix(strings.ToLower(folderName), trickplayFolderSuffix) {
		return "", false
	}
	return folderName[:len(folderName)-len(trickplayFolderSuffix)], true
}

// trickplayResolutionPattern matches the subfolder Jellyfin generates inside a
// trickplay folder, one per resolution: the image width, then the tile grid each
// sheet is packed in. It is the same expression Jellyfin reads these folders
// back with, spaces around the dash included.
var trickplayResolutionPattern = regexp.MustCompile(`^(\d+) - (\d+)x(\d+)$`)

// parseTrickplayResolutionFolder reads the width and tile grid out of a
// trickplay resolution folder name, such as "320 - 10x10".
func parseTrickplayResolutionFolder(folderName string) (width, tileWidth, tileHeight int, ok bool) {
	match := trickplayResolutionPattern.FindStringSubmatch(folderName)
	if match == nil {
		return 0, 0, 0, false
	}
	// Every group is \d+ and so parses, except for a number too large to hold —
	// which is not a resolution, and is treated as an unrecognized folder.
	width, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, 0, 0, false
	}
	tileWidth, err = strconv.Atoi(match[2])
	if err != nil {
		return 0, 0, 0, false
	}
	tileHeight, err = strconv.Atoi(match[3])
	if err != nil {
		return 0, 0, 0, false
	}
	return width, tileWidth, tileHeight, true
}

// ignoredFolders are scraper and platform caches that describe nothing about
// the media itself.
var ignoredFolders = map[string]bool{
	"extrafanart":  true,
	"extrathumbs":  true,
	"@eadir":       true,
	".actors":      true,
	".appledouble": true,
	"lost+found":   true,
}

// isIgnoredFolder reports folders skipped outright, including any hidden
// folder. Jellyfin's .trickplay folders used to be skipped here too; they are
// now walked and their tiles recorded, which is what trickplayFolderBaseName
// above exists for.
func isIgnoredFolder(folderName string) bool {
	if ignoredFolders[strings.ToLower(folderName)] {
		return true
	}
	return strings.HasPrefix(folderName, ".")
}

// ignoredFiles are platform metadata files, never media.
var ignoredFiles = map[string]bool{
	"thumbs.db":   true,
	".ds_store":   true,
	"desktop.ini": true,
}

// isIgnoredFile reports files skipped outright, including hidden files and the
// AppleDouble "._" resource forks that shadow every real file on some shares.
func isIgnoredFile(fileName string) bool {
	if ignoredFiles[strings.ToLower(fileName)] {
		return true
	}
	return strings.HasPrefix(fileName, ".")
}

// Artwork names and external-subtitle flag tokens used to live here. They are
// now expressed as regular expressions in the sidecar classification table,
// which is configuration rather than Go — see appconfig.DefaultSidecarTypes.

// The stereoscopic layout flags (hsbs, fsbs, htab, ftab, mvc) and the
// multi-part tokens (cd, dvd, part, pt, disc, disk) are enumerated directly in
// threeDPattern and stackPattern in parse.go, since both are only ever matched
// as a suffix rather than looked up.
