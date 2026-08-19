package mediascan

// This is the result of one directory scan.  Its purpose is to record the top level metadata structure which is
// stored in the database and used to populate the nfo files on write.  Its also used to identify all the sidecar files
// in the directory.

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"Metarr/internal/metadata"
)

// ScanResult is everything one directory scan produced.
type ScanResult struct {
	Directory  *TVSeries   `bson:"directory" json:"directory"`
	MediaFiles []MediaFile `bson:"media_files" json:"media_files"`
}

// TVSeries is one media item directory, stored with record_type
// "directory". Its ID is left zero by a scan: MongoDB assigns it on insert, and
// the repository matches on Path so the assigned ID stays stable across
// rescans.
type TVSeries struct {
	ID           bson.ObjectID      `bson:"_id,omitempty" json:"id"`
	RecordType   RecordType         `bson:"record_type" json:"record_type"`
	Path         string             `bson:"path" json:"path"`
	ScanRootPath string             `bson:"scan_root_path" json:"scan_root_path"`
	Type         DirectoryType      `bson:"type" json:"type"`
	FolderName   string             `bson:"folder_name" json:"folder_name"`
	Metadata     *metadata.Metadata `bson:"metadata,omitempty" json:"metadata,omitempty"`
	ScannedAt    time.Time          `bson:"scanned_at" json:"scanned_at"`

	Seasons []TVSeason `bson:"seasons,omitempty" json:"seasons,omitempty"`

	Warnings []string      `bson:"warnings,omitempty" json:"warnings,omitempty"`
	Sidecars []SidecarFile `bson:"sidecars" json:"sidecars"`
}

type TVSeason struct {
	SeasonNumber int           `bson:"season_number" json:"season_number"`
	FolderName   string        `bson:"folder_name" json:"folder_name"`
	Sidecars     []SidecarFile `bson:"sidecars" json:"sidecars"`
}

// MediaFile is one playable file — a movie, an episode, or a music video —
// stored with record_type "media_file" and linked to its directory by
// DirectoryID. Sidecars belonging to this specific file travel with it rather
// than with the directory.
type MediaFile struct {
	ID            bson.ObjectID             `bson:"_id,omitempty" json:"id"`
	RecordType    RecordType                `bson:"record_type" json:"record_type"`
	DirectoryID   bson.ObjectID             `bson:"directory_id,omitempty" json:"directory_id"`
	DirectoryPath string                    `bson:"directory_path" json:"directory_path"`
	ScanRootPath  string                    `bson:"scan_root_path" json:"scan_root_path"`
	Type          DirectoryType             `bson:"type" json:"type"`
	Path          string                    `bson:"path" json:"path"`
	RelativePath  string                    `bson:"relative_path" json:"relative_path"`
	FileName      string                    `bson:"file_name" json:"file_name"`
	Extension     string                    `bson:"extension" json:"extension"`
	SizeBytes     int64                     `bson:"size_bytes" json:"size_bytes"`
	ModifiedAt    time.Time                 `bson:"modified_at" json:"modified_at"`
	ScannedAt     time.Time                 `bson:"scanned_at" json:"scanned_at"`
	Video         *metadata.VideoAttributes `bson:"video,omitempty" json:"video,omitempty"`
	// Stat is the filesystem's view of the file — ownership, permissions, inode,
	// timestamps — as recorded at scan time. FileInfo below is the codec's view;
	// the two describe different things despite the similar names.
	Stat     *FileStat          `bson:"stat,omitempty" json:"stat,omitempty"`
	FileInfo *FileInfo          `bson:"file_info,omitempty" json:"file_info,omitempty"`
	Metadata *metadata.Metadata `bson:"metadata,omitempty" json:"metadata,omitempty"`
	Sidecars []SidecarFile      `bson:"sidecars" json:"sidecars"`

	Warnings []string `bson:"warnings,omitempty" json:"warnings,omitempty"`
}

// SidecarFile is one non-media file attached to a season, classified against
// the sidecar type registry below. Category is stored alongside Type —
// denormalized on purpose — so a query can select a whole class of files
// directly, e.g. {"seasons.sidecars.category": "image"}.
type SidecarFile struct {
	RelativePath string          `bson:"relative_path" json:"relative_path"`
	FileName     string          `bson:"file_name" json:"file_name"`
	Extension    string          `bson:"extension" json:"extension"`
	SizeBytes    int64           `bson:"size_bytes" json:"size_bytes"`
	ModifiedAt   time.Time       `bson:"modified_at" json:"modified_at"`
	Type         SidecarType     `bson:"sidecar_type" json:"sidecar_type"`
	Category     SidecarCategory `bson:"category" json:"category"`
	// Stat is what the filesystem reports about the file itself: ownership,
	// permissions, inode and the full set of timestamps.
	Stat *FileStat `bson:"stat,omitempty" json:"stat,omitempty"`
	// Image is the codec and dimensions read from the file's header, present
	// only on sidecars whose extension marks them as artwork.
	Image *ImageInfo `bson:"image,omitempty" json:"image,omitempty"`
	// Trickplay places a scrubbing preview tile within its media file's
	// previews, and is present only on those.
	Trickplay *TrickplayInfo `bson:"trickplay,omitempty" json:"trickplay,omitempty"`
}

// TrickplayInfo places one tile sheet within its media file's scrubbing
// previews: the resolution Jellyfin generated it at, the grid each sheet packs
// its thumbnails into, and which sheet in the sequence this one is. All three
// come from the folder layout, which is the only thing that describes a tile —
// the file itself is called "0.jpg".
type TrickplayInfo struct {
	Width      int `bson:"width" json:"width"`
	TileWidth  int `bson:"tile_width" json:"tile_width"`
	TileHeight int `bson:"tile_height" json:"tile_height"`
	// TileIndex is nil for a file inside a resolution folder that is not a
	// numbered tile, which is different from being the first one.
	TileIndex *int `bson:"tile_index,omitempty" json:"tile_index,omitempty"`
}

// FileInfo wraps the technical stream description.
type FileInfo struct {
	StreamDetails *StreamDetails `xml:"streamdetails,omitempty" bson:"stream_details,omitempty" json:"stream_details,omitempty"`
}

// StreamDetails describes the media streams inside the video file.
type StreamDetails struct {
	Video    []VideoStream    `xml:"video,omitempty" bson:"video,omitempty" json:"video,omitempty"`
	Audio    []AudioStream    `xml:"audio,omitempty" bson:"audio,omitempty" json:"audio,omitempty"`
	Subtitle []SubtitleStream `xml:"subtitle,omitempty" bson:"subtitle,omitempty" json:"subtitle,omitempty"`
}

// VideoStream is one video track.
type VideoStream struct {
	Codec             string `xml:"codec,omitempty" bson:"codec,omitempty" json:"codec,omitempty"`
	Aspect            string `xml:"aspect,omitempty" bson:"aspect,omitempty" json:"aspect,omitempty"`
	Width             string `xml:"width,omitempty" bson:"width,omitempty" json:"width,omitempty"`
	Height            string `xml:"height,omitempty" bson:"height,omitempty" json:"height,omitempty"`
	DurationInSeconds string `xml:"durationinseconds,omitempty" bson:"duration_in_seconds,omitempty" json:"duration_in_seconds,omitempty"`
	StereoMode        string `xml:"stereomode,omitempty" bson:"stereo_mode,omitempty" json:"stereo_mode,omitempty"`
	HDRType           string `xml:"hdrtype,omitempty" bson:"hdr_type,omitempty" json:"hdr_type,omitempty"`
}

// AudioStream is one audio track.
type AudioStream struct {
	Codec    string `xml:"codec,omitempty" bson:"codec,omitempty" json:"codec,omitempty"`
	Language string `xml:"language,omitempty" bson:"language,omitempty" json:"language,omitempty"`
	Channels string `xml:"channels,omitempty" bson:"channels,omitempty" json:"channels,omitempty"`
}

// SubtitleStream is one embedded subtitle track.
type SubtitleStream struct {
	Language string `xml:"language,omitempty" bson:"language,omitempty" json:"language,omitempty"`
}
