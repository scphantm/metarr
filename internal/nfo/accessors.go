package nfo

import (
	"encoding/xml"
	"strconv"
	"strings"
)

// The numeric-looking tags in these documents are modelled as strings, because
// real NFO files routinely contain empty tags (<runtime></runtime>) and
// non-numeric junk, and encoding/xml fails outright unmarshaling those into an
// int. Keeping the raw text means a file round-trips exactly as found; these
// accessors give callers the parsed value where they actually need one. The
// bool reports whether the tag held a usable number at all.

// RuntimeMinutes returns the movie's runtime in minutes.
func (m Movie) RuntimeMinutes() (int, bool) { return parseIntTag(m.Runtime) }

// UserRatingValue returns the movie's user rating.
func (m Movie) UserRatingValue() (float64, bool) { return parseFloatTag(m.UserRating) }

// RuntimeMinutes returns the show's runtime in minutes.
func (s TVShow) RuntimeMinutes() (int, bool) { return parseIntTag(s.Runtime) }

// SeasonNumber returns the season this episode belongs to.
func (e EpisodeDetails) SeasonNumber() (int, bool) { return parseIntTag(e.Season) }

// EpisodeNumber returns this episode's number within its season.
func (e EpisodeDetails) EpisodeNumber() (int, bool) { return parseIntTag(e.Episode) }

// RuntimeMinutes returns the episode's runtime in minutes.
func (e EpisodeDetails) RuntimeMinutes() (int, bool) { return parseIntTag(e.Runtime) }

// TrackNumber returns the music video's track number.
func (v MusicVideo) TrackNumber() (int, bool) { return parseIntTag(v.Track) }

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

// restoreUnknownElementNames repopulates XMLName from Name for any preserved
// element that lost it.
//
// XMLName is tagged bson:"-" because an xml.Name is meaningless outside XML, so
// a Document loaded back out of Mongo arrives with XMLName empty — and
// encoding/xml needs it to know what to call the element. Without this, a
// preserved unknown tag would be silently dropped or mis-emitted on the first
// write of a document that had been through the database.
func restoreUnknownElementNames(elements []UnknownElement) {
	for i := range elements {
		if elements[i].XMLName.Local == "" {
			elements[i].XMLName = xml.Name{Local: elements[i].Name}
		}
	}
}

// captureUnknownElementNames mirrors XMLName into Name so the element survives
// being stored somewhere that doesn't keep xml.Name.
func captureUnknownElementNames(elements []UnknownElement) {
	for i := range elements {
		if elements[i].Name == "" {
			elements[i].Name = elements[i].XMLName.Local
		}
	}
}
