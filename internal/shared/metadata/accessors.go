package metadata

import (
	"strconv"
	"strings"
	"time"

	"github.com/oapi-codegen/runtime/types"
)

// dateLayouts are the shapes a date tag turns up in on disk, most specific
// first. Kodi writes <premiered> and <aired> as a bare date but <dateadded> with
// a time attached, and files touched by other tools show up in RFC 3339, so all
// three are accepted rather than only the one the specification names.
var dateLayouts = []string{
	"2006-01-02",
	"2006-01-02 15:04:05",
	time.RFC3339,
}

// ParseDate reads a date tag from an NFO file. Anything unparseable — an empty
// tag, junk, or a partial date — yields the zero Date rather than an error: a
// scan must not fail over one badly written field, and the zero value is what
// FormatDate turns back into an absent tag.
func ParseDate(raw string) types.Date {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return types.Date{}
	}
	for _, layout := range dateLayouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return types.Date{Time: parsed}
		}
	}
	return types.Date{}
}

// FormatDate renders a date back into the form NFO files use. A zero Date
// becomes the empty string, which the document structs tag omitempty so the
// element is left out entirely rather than written blank.
//
// Note that a value read from a tag carrying a time of day — <dateadded>
// usually does — comes back as a bare date, since Date models a day rather than
// an instant.
func FormatDate(date types.Date) string {
	if date.IsZero() {
		return ""
	}
	return date.Format("2006-01-02")
}

// The numeric-looking tags from an NFO file are modelled as strings, because
// real files routinely contain empty tags (<runtime></runtime>) and
// non-numeric junk. These accessors give callers the parsed value where they
// actually need one; the bool reports whether the tag held a usable number at
// all.

// RuntimeMinutes returns the runtime in minutes.
func (m Metadata) RuntimeMinutes() (int, bool) { return parseIntTag(m.Runtime) }

// UserRatingValue returns the user rating.
func (m Metadata) UserRatingValue() (float64, bool) { return parseFloatTag(m.UserRating) }

// SeasonNumber returns the episode's season, when this record is an episode.
func (m Metadata) SeasonNumber() (int, bool) {
	if m.Episode == nil {
		return 0, false
	}
	return parseIntTag(m.Episode.Season)
}

// EpisodeNumber returns the episode's number within its season, when this
// record is an episode.
func (m Metadata) EpisodeNumber() (int, bool) {
	if m.Episode == nil {
		return 0, false
	}
	return parseIntTag(m.Episode.Episode)
}

// TrackNumber returns the music video's track number, when this record is a
// music video.
func (m Metadata) TrackNumber() (int, bool) {
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
