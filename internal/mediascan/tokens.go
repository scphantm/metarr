package mediascan

import "strings"

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

// isIgnoredFolder reports folders skipped outright, including any hidden folder
// and Jellyfin's generated .trickplay directories.
func isIgnoredFolder(folderName string) bool {
	lowered := strings.ToLower(folderName)
	if ignoredFolders[lowered] {
		return true
	}
	if strings.HasSuffix(lowered, ".trickplay") {
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

// Image type names recorded on artwork files.
const (
	ImagePoster   = "poster"
	ImageBackdrop = "backdrop"
	ImageBanner   = "banner"
	ImageLogo     = "logo"
	ImageThumb    = "thumb"
	ImageDisc     = "disc"
	ImageClearArt = "clearart"
)

// imageTypeTokens maps the bare artwork filenames both servers recognize onto
// one image type each.
var imageTypeTokens = map[string]string{
	"poster":     ImagePoster,
	"folder":     ImagePoster,
	"cover":      ImagePoster,
	"default":    ImagePoster,
	"movie":      ImagePoster,
	"show":       ImagePoster,
	"backdrop":   ImageBackdrop,
	"fanart":     ImageBackdrop,
	"background": ImageBackdrop,
	"art":        ImageBackdrop,
	"banner":     ImageBanner,
	"logo":       ImageLogo,
	"clearlogo":  ImageLogo,
	"thumb":      ImageThumb,
	"landscape":  ImageThumb,
	"disc":       ImageDisc,
	"discart":    ImageDisc,
	"cdart":      ImageDisc,
	"clearart":   ImageClearArt,
}

// Subtitle flag tokens, per the Jellyfin external-subtitle naming rules.
var (
	subtitleForcedTokens  = map[string]bool{"forced": true, "foreign": true}
	subtitleDefaultTokens = map[string]bool{"default": true}
	subtitleHearingTokens = map[string]bool{"sdh": true, "cc": true, "hi": true}
)

// The stereoscopic layout flags (hsbs, fsbs, htab, ftab, mvc) and the
// multi-part tokens (cd, dvd, part, pt, disc, disk) are enumerated directly in
// threeDPattern and stackPattern in parse.go, since both are only ever matched
// as a suffix rather than looked up.
