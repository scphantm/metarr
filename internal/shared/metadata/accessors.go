package metadata

import (
	"strconv"
	"strings"
	"time"
)

// dateLayouts are the shapes a date tag turns up in on disk, most specific
// first. Kodi writes <premiered> and <aired> as a bare date but <dateadded>
// with a time attached, and files touched by other tools show up in RFC 3339,
// so all three are accepted rather than only the one the specification names.
var dateLayouts = []string{
	"2006-01-02",
	"2006-01-02 15:04:05",
	time.RFC3339,
}

// ParseDate normalizes a date tag from an NFO file to the bare "2006-01-02"
// form the model stores. Anything unparseable — an empty tag, junk, or a
// partial date — yields "" rather than an error: a scan must not fail over
// one badly written field, and "" is what FormatDate turns back into an
// absent tag.
//
// A value read from a tag carrying a time of day — <dateadded> usually does
// — comes back as a bare date, since the model records a day, not an instant.
func ParseDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, layout := range dateLayouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return ""
}

// FormatDate renders a stored date back into the form NFO files use. The
// stored value is already normalized by ParseDate, so this only guards
// against a stray value: a non-date string becomes "", which the document
// structs tag omitempty so the element is left out rather than written blank.
func FormatDate(date string) string {
	return ParseDate(date)
}

// The numeric-looking tags from an NFO file are modelled as strings, because
// real files routinely contain empty tags (<runtime></runtime>) and
// non-numeric junk. These helpers give callers the parsed value where they
// actually need one; the bool reports whether the tag held a usable number.

// RuntimeMinutes returns the runtime in minutes.
func RuntimeMinutes(m *Metadata) (int, bool) { return parseIntTag(m.Runtime) }

// UserRatingValue returns the user rating.
func UserRatingValue(m *Metadata) (float64, bool) { return parseFloatTag(m.UserRating) }

// SeasonNumber returns the episode's season, when this record is an episode.
func SeasonNumber(m *Metadata) (int, bool) {
	if m.Episode == nil {
		return 0, false
	}
	return parseIntTag(m.Episode.Season)
}

// EpisodeNumber returns the episode's number within its season, when this
// record is an episode.
func EpisodeNumber(m *Metadata) (int, bool) {
	if m.Episode == nil {
		return 0, false
	}
	return parseIntTag(m.Episode.Episode)
}

// TrackNumber returns the music video's track number, when this record is a
// music video.
func TrackNumber(m *Metadata) (int, bool) {
	if m.MusicVideo == nil {
		return 0, false
	}
	return parseIntTag(m.MusicVideo.Track)
}

func parseIntTag(raw string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return value, true
}

func parseFloatTag(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
