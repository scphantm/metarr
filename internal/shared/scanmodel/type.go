// Package mediascan walks a media item directory and classifies everything in
// it according to the Jellyfin and Plex naming conventions, producing records
// ready to store in MongoDB.
//
// The unit of work is one item directory — a single movie, series, or music
// video folder — which is what a caller scanning a library iterates over. A
// scan yields one directory record plus one record per media file, where a
// "media file" is the playable content itself: a movie file, an episode file, or
// a music video file. Trailers, themes, artwork, subtitles and NFO sidecars are
// not media files; they are indexed on the record they belong to.
//
// Nothing in this package touches a database or a network. It reads file names
// and the filesystem's own stat record, parses the contents of .nfo sidecars via
// internal/nfo, and reads the header — not the pixels — of artwork sidecars for
// their codec and dimensions. That makes the whole thing testable against
// synthetic directory trees, which is where the naming rules are actually
// pinned down.
package scanmodel

import "fmt"

// DirectoryType is the kind of media a directory holds, declared by
// configuration rather than guessed.
type DirectoryType string

const (
	TypeMovie      DirectoryType = "movie"
	TypeTV         DirectoryType = "tv"
	TypeMusicVideo DirectoryType = "music_video"
)

// validDirectoryTypes is ordered so error messages list the vocabulary
// predictably.
var validDirectoryTypes = []DirectoryType{TypeMovie, TypeTV, TypeMusicVideo}

// ParseDirectoryType validates a scan_type value from the directory scanner
// configuration. It is the single source of truth for the vocabulary, shared by
// the config handlers that store a scan directory and the listener that acts on
// one, so an unscannable value can't be saved in the first place.
func ParseDirectoryType(scanType string) (DirectoryType, error) {
	for _, candidate := range validDirectoryTypes {
		if DirectoryType(scanType) == candidate {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("mediascan: unknown scan type %q, expected one of %s", scanType, ValidDirectoryTypesText())
}

// ValidDirectoryTypesText renders the accepted scan types for error messages.
func ValidDirectoryTypesText() string {
	names := make([]string, 0, len(validDirectoryTypes))
	for _, directoryType := range validDirectoryTypes {
		names = append(names, string(directoryType))
	}
	return joinQuoted(names)
}

func joinQuoted(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	switch len(quoted) {
	case 0:
		return ""
	case 1:
		return quoted[0]
	default:
		result := ""
		for i, value := range quoted {
			switch {
			case i == 0:
				result = value
			case i == len(quoted)-1:
				result += " or " + value
			default:
				result += ", " + value
			}
		}
		return result
	}
}

// RecordType discriminates the two kinds of document stored in the
// local_directory collection.
type RecordType string

const (
	RecordTypeTVSeries  RecordType = "tvseries"
	RecordTypeMediaFile RecordType = "media_file"
)
