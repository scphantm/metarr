package mediascan

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"Metarr/internal/nfo"
)

// LocalDirectory is one media item directory, stored with record_type
// "directory". Its ID is left zero by a scan: MongoDB assigns it on insert, and
// the repository matches on Path so the assigned ID stays stable across
// rescans.
type LocalDirectory struct {
	ID            bson.ObjectID `bson:"_id,omitempty" json:"id"`
	RecordType    RecordType    `bson:"record_type" json:"record_type"`
	Path          string        `bson:"path" json:"path"`
	ScanRootPath  string        `bson:"scan_root_path" json:"scan_root_path"`
	Type          DirectoryType `bson:"type" json:"type"`
	FolderName    string        `bson:"folder_name" json:"folder_name"`
	Title         string        `bson:"title" json:"title"`
	Year          int           `bson:"year,omitempty" json:"year,omitempty"`
	Artist        string        `bson:"artist,omitempty" json:"artist,omitempty"`
	ExternalLinks []nfo.Link    `bson:"external_links" json:"external_links"`
	ScannedAt     time.Time     `bson:"scanned_at" json:"scanned_at"`

	// Files holds everything in the tree that is not a media file: artwork,
	// directory-level NFO sidecars, theme music, trailers and other extras,
	// disc structure, and anything unrecognized.
	Files    []DirectoryFile `bson:"files" json:"files"`
	Seasons  []SeasonSummary `bson:"seasons,omitempty" json:"seasons,omitempty"`
	Warnings []string        `bson:"warnings,omitempty" json:"warnings,omitempty"`
}

// MediaFile is one playable file — a movie, an episode, or a music video —
// stored with record_type "media_file" and linked to its directory by
// DirectoryID. Sidecars belonging to this specific file travel with it rather
// than with the directory.
type MediaFile struct {
	ID            bson.ObjectID `bson:"_id,omitempty" json:"id"`
	RecordType    RecordType    `bson:"record_type" json:"record_type"`
	DirectoryID   bson.ObjectID `bson:"directory_id,omitempty" json:"directory_id"`
	DirectoryPath string        `bson:"directory_path" json:"directory_path"`
	ScanRootPath  string        `bson:"scan_root_path" json:"scan_root_path"`
	Type          DirectoryType `bson:"type" json:"type"`

	Path         string    `bson:"path" json:"path"`
	RelativePath string    `bson:"relative_path" json:"relative_path"`
	FileName     string    `bson:"file_name" json:"file_name"`
	Extension    string    `bson:"extension" json:"extension"`
	SizeBytes    int64     `bson:"size_bytes" json:"size_bytes"`
	ModifiedAt   time.Time `bson:"modified_at" json:"modified_at"`
	ScannedAt    time.Time `bson:"scanned_at" json:"scanned_at"`

	Video *VideoAttributes `bson:"video,omitempty" json:"video,omitempty"`

	NFO       *NFOAttributes  `bson:"nfo,omitempty" json:"nfo,omitempty"`
	Subtitles []DirectoryFile `bson:"subtitles,omitempty" json:"subtitles,omitempty"`
	Images    []DirectoryFile `bson:"images,omitempty" json:"images,omitempty"`

	// EpisodeIDs holds this episode's own external provider identifiers, kept
	// separate from the directory's series-level external_links.
	EpisodeIDs []nfo.Link `bson:"episode_ids,omitempty" json:"episode_ids,omitempty"`

	Warnings []string `bson:"warnings,omitempty" json:"warnings,omitempty"`
}

// SeasonSummary is a queryable index over a series' media files, not a second
// copy of them.
type SeasonSummary struct {
	SeasonNumber int    `bson:"season_number" json:"season_number"`
	FolderName   string `bson:"folder_name" json:"folder_name"`
	EpisodeCount int    `bson:"episode_count" json:"episode_count"`
}

// DirectoryFile is a non-media file, embedded in whichever record owns it: the
// directory record for directory-wide files, or a media file record for that
// file's own sidecars.
type DirectoryFile struct {
	RelativePath string    `bson:"relative_path" json:"relative_path"`
	FileName     string    `bson:"file_name" json:"file_name"`
	Extension    string    `bson:"extension" json:"extension"`
	SizeBytes    int64     `bson:"size_bytes" json:"size_bytes"`
	ModifiedAt   time.Time `bson:"modified_at" json:"modified_at"`
	Role         FileRole  `bson:"role" json:"role"`

	Subtitle *SubtitleAttributes `bson:"subtitle,omitempty" json:"subtitle,omitempty"`
	Image    *ImageAttributes    `bson:"image,omitempty" json:"image,omitempty"`
	NFO      *NFOAttributes      `bson:"nfo,omitempty" json:"nfo,omitempty"`
	Video    *VideoAttributes    `bson:"video,omitempty" json:"video,omitempty"`
}

// FileRole is what a non-media file is. There is deliberately no
// "primary_video" role: a playable video is a MediaFile record, which keeps the
// two record kinds exhaustive and non-overlapping.
type FileRole string

const (
	RoleExtraVideo    FileRole = "extra_video"
	RoleSubtitle      FileRole = "subtitle"
	RoleNFO           FileRole = "nfo"
	RoleImage         FileRole = "image"
	RoleThemeMusic    FileRole = "theme_music"
	RoleDiscStructure FileRole = "disc_structure"
	RoleUnknown       FileRole = "unknown"
)

// VideoAttributes is everything parsed out of a video file's name.
type VideoAttributes struct {
	Title        string `bson:"title,omitempty" json:"title,omitempty"`
	EpisodeTitle string `bson:"episode_title,omitempty" json:"episode_title,omitempty"`
	Year         int    `bson:"year,omitempty" json:"year,omitempty"`

	// SeasonNumber is nil when it could not be resolved, which is different
	// from season zero (specials).
	SeasonNumber   *int   `bson:"season_number,omitempty" json:"season_number,omitempty"`
	EpisodeNumbers []int  `bson:"episode_numbers,omitempty" json:"episode_numbers,omitempty"`
	AirDate        string `bson:"air_date,omitempty" json:"air_date,omitempty"`
	IsSpecial      bool   `bson:"is_special,omitempty" json:"is_special,omitempty"`

	Edition      string `bson:"edition,omitempty" json:"edition,omitempty"`
	VersionLabel string `bson:"version_label,omitempty" json:"version_label,omitempty"`
	StackType    string `bson:"stack_type,omitempty" json:"stack_type,omitempty"`
	StackNumber  int    `bson:"stack_number,omitempty" json:"stack_number,omitempty"`
	ThreeDFormat string `bson:"three_d_format,omitempty" json:"three_d_format,omitempty"`

	// ExtraType and ExtraSource are set only on extra videos.
	ExtraType   string `bson:"extra_type,omitempty" json:"extra_type,omitempty"`
	ExtraSource string `bson:"extra_source,omitempty" json:"extra_source,omitempty"`
}

// Extra sources, recording how a video was identified as an extra.
const (
	ExtraSourceFolder = "folder"
	ExtraSourceSuffix = "suffix"
)

// SubtitleAttributes is everything parsed out of an external subtitle's name.
type SubtitleAttributes struct {
	TargetBaseName  string `bson:"target_base_name,omitempty" json:"target_base_name,omitempty"`
	Language        string `bson:"language,omitempty" json:"language,omitempty"`
	Title           string `bson:"title,omitempty" json:"title,omitempty"`
	Forced          bool   `bson:"forced,omitempty" json:"forced,omitempty"`
	Default         bool   `bson:"default,omitempty" json:"default,omitempty"`
	HearingImpaired bool   `bson:"hearing_impaired,omitempty" json:"hearing_impaired,omitempty"`
}

// ImageAttributes is everything parsed out of an artwork file's name.
type ImageAttributes struct {
	ImageType string `bson:"image_type" json:"image_type"`
	// Index distinguishes numbered artwork such as backdrop-1 and backdrop2.
	Index          int    `bson:"index,omitempty" json:"index,omitempty"`
	Scope          string `bson:"scope" json:"scope"`
	SeasonNumber   *int   `bson:"season_number,omitempty" json:"season_number,omitempty"`
	TargetBaseName string `bson:"target_base_name,omitempty" json:"target_base_name,omitempty"`
}

// NFOAttributes records where an NFO sidecar sits in the tree, plus its parsed
// contents. The contents are read during the scan so one pass makes the whole
// library's metadata queryable.
type NFOAttributes struct {
	Scope          string `bson:"scope" json:"scope"`
	SeasonNumber   *int   `bson:"season_number,omitempty" json:"season_number,omitempty"`
	TargetBaseName string `bson:"target_base_name,omitempty" json:"target_base_name,omitempty"`

	Kind       nfo.DocumentKind     `bson:"kind,omitempty" json:"kind,omitempty"`
	Movie      *nfo.Movie           `bson:"movie,omitempty" json:"movie,omitempty"`
	TVShow     *nfo.TVShow          `bson:"tvshow,omitempty" json:"tvshow,omitempty"`
	Episodes   []nfo.EpisodeDetails `bson:"episodes,omitempty" json:"episodes,omitempty"`
	MusicVideo *nfo.MusicVideo      `bson:"musicvideo,omitempty" json:"musicvideo,omitempty"`
	// ParseError records why an NFO could not be read. A malformed sidecar is
	// reported here rather than failing the scan.
	ParseError string `bson:"parse_error,omitempty" json:"parse_error,omitempty"`
}

// Scopes describing which level of the tree a sidecar applies to.
const (
	ScopeDirectory = "directory"
	ScopeSeason    = "season"
	ScopeVideo     = "video"
)

// ScanResult is everything one directory scan produced.
type ScanResult struct {
	Directory  *LocalDirectory `bson:"directory" json:"directory"`
	MediaFiles []MediaFile     `bson:"media_files" json:"media_files"`
}
