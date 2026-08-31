// This file holds the closed vocabularies that tag a scan record: the kind of
// media a directory holds (DirectoryType) and the two document kinds stored in
// the local_directory collection (RecordType). They are free strings on the
// generated messages; the Parse* helpers here are the single source of truth
// for the values they may take, so an unusable one cannot be stored.
package scanmodel

import "fmt"

// DirectoryType is the kind of media a directory holds, declared by
// configuration rather than guessed. It is a free string on the generated
// record messages; ParseDirectoryType is the single source of truth for the
// values it may take.
type DirectoryType string

// The values DirectoryType takes. Untyped so they compare directly against the
// plain string field on the generated messages, the same way the metadata
// package's Kind/Scope constants do.
const (
	TypeMovie      = "movie"
	TypeTV         = "tv"
	TypeMusicVideo = "music_video"
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
// local_directory collection. It is a free string on the generated record
// messages; these are the values it takes, untyped so they compare directly
// against that field and against a BSON query value.
const (
	RecordTypeTVSeries  = "tvseries"
	RecordTypeMediaFile = "media_file"
)
