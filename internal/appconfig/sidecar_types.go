package appconfig

// This file holds the sidecar classification table: the rules that decide what
// a non-media file found next to a movie or episode actually is. It lives in
// configuration rather than in Go so a library with unusual naming can be
// accommodated by editing a document, not by editing and redeploying the
// scanner.
//
// The vocabulary the table draws on comes from the Jellyfin and Plex media
// organization specifications:
//
//	https://jellyfin.org/docs/general/server/media/movies
//	https://jellyfin.org/docs/general/server/media/shows/
//	https://support.plex.tv/articles/naming-and-organizing-your-tv-show-files/

// SidecarTypeDefinition is one entry in the sidecar classification table.
//
// Patterns are Go regular expressions matched against a file's base name — the
// name with its extension removed — so a pattern never has to spell out
// extensions itself. They are written case-insensitive with an inline (?i).
//
// Extensions gates the match to those lowercase, dot-prefixed extensions, which
// is what stops the "poster" entry from claiming poster.txt. An empty list
// accepts any extension.
type SidecarTypeDefinition struct {
	// ID is the stable handle the config API edits an entry by. It is minted
	// once and never reused, so an entry survives being renamed or retyped
	// without losing its place in the evaluation sequence.
	ID string `bson:"id" json:"id"`

	// Type is what gets written onto every file classified as this kind, so it
	// is unique across the table even though ID is the API's handle.
	Type     string `bson:"type" json:"type"`
	Category string `bson:"category" json:"category"`

	// Order is the evaluation sequence: the scanner takes the first enabled
	// entry that accepts a file, so narrower entries belong ahead of the
	// catch-alls. Position in the stored array means nothing.
	//
	// Zero means the entry is disabled — kept in the table, still editable, but
	// never evaluated. That is how a built-in type is switched off without
	// losing its patterns.
	//
	// Order is changed only through the dedicated ordering endpoint, never by
	// editing an entry, because uniqueness is a property of the whole table
	// rather than of any one row.
	Order int `bson:"order" json:"order"`

	Patterns   []string `bson:"patterns" json:"patterns"`
	Extensions []string `bson:"extensions" json:"extensions"`
}

// Extension sets shared by the default entries below, declared once so a type
// that accepts "any image" says so by reference rather than by repetition. Each
// default entry gets its own copy, so an edit to one stored entry can never
// reach another.
var (
	imageExtensions    = []string{".jpg", ".jpeg", ".png", ".tbn", ".webp", ".gif", ".bmp"}
	videoExtensions    = []string{".mkv", ".mp4", ".avi", ".m4v", ".mov", ".wmv", ".mpg", ".mpeg", ".ts", ".webm", ".flv", ".iso"}
	audioExtensions    = []string{".mp3", ".flac", ".m4a", ".wav", ".ogg", ".opus", ".aac"}
	subtitleExtensions = []string{".srt", ".ass", ".ssa", ".sub", ".idx", ".vtt", ".smi", ".sup"}
	discExtensions     = []string{".ifo", ".bup", ".vob", ".bdmv", ".m2ts", ".mpls", ".clpi"}
)

// DefaultSidecarTypes returns the built-in classification table, seeded into a
// fresh configuration and restored by the reset endpoint. Callers get their own
// copy: the returned slices are never shared with a previous caller, so a stored
// configuration cannot be mutated through one.
func DefaultSidecarTypes() []SidecarTypeDefinition {
	defaults := []SidecarTypeDefinition{
		{
			ID:         "d6dd8837-3f91-4aa5-99a9-d2a8f35608e7",
			Type:       "poster",
			Category:   "image",
			Order:      10,
			Patterns:   []string{`(?i)^(poster|folder|cover|default|movie)(?:[-._ ]?\d+)?$`, `(?i)[-._ ]poster(?:[-._ ]?\d+)?$`, `(?i)^season\d+([-._ ]?poster)?$`, `(?i)^season[-._ ]?specials([-._ ]?poster)?$`},
			Extensions: imageExtensions,
		},
		{
			ID:         "32a37ee3-4f16-443a-9d94-3623216e3f1f",
			Type:       "fanart",
			Category:   "image",
			Order:      20,
			Patterns:   []string{`(?i)^(fanart|backdrop|background|art)(?:[-._ ]?\d+)?$`, `(?i)[-._ ](fanart|backdrop|background)(?:[-._ ]?\d+)?$`},
			Extensions: imageExtensions,
		},
		{
			ID:         "d760c51d-d54d-4edc-ab95-eb3495ba0444",
			Type:       "banner",
			Category:   "image",
			Order:      30,
			Patterns:   []string{`(?i)^banner(?:[-._ ]?\d+)?$`, `(?i)[-._ ]banner(?:[-._ ]?\d+)?$`, `(?i)^season\d+[-._ ]?banner$`},
			Extensions: imageExtensions,
		},
		{
			ID:         "bc68f4a4-1b0b-4e6d-850a-0de6c248740b",
			Type:       "clearlogo",
			Category:   "image",
			Order:      40,
			Patterns:   []string{`(?i)^(clearlogo|logo)(?:[-._ ]?\d+)?$`, `(?i)[-._ ](clearlogo|logo)(?:[-._ ]?\d+)?$`},
			Extensions: imageExtensions,
		},
		{
			ID:         "2c2d8be2-0416-47b1-a0ba-cf5b5b16710f",
			Type:       "clearart",
			Category:   "image",
			Order:      50,
			Patterns:   []string{`(?i)^clearart(?:[-._ ]?\d+)?$`, `(?i)[-._ ]clearart(?:[-._ ]?\d+)?$`},
			Extensions: imageExtensions,
		},
		{
			ID:         "547f0c65-6f82-4391-9453-3ff312d1d7ab",
			Type:       "discart",
			Category:   "image",
			Order:      60,
			Patterns:   []string{`(?i)^(discart|disc|cdart)(?:[-._ ]?\d+)?$`, `(?i)[-._ ](discart|cdart)(?:[-._ ]?\d+)?$`},
			Extensions: imageExtensions,
		},
		{
			ID:         "7175d6ee-ce4d-4f0c-91bc-e04a1838a434",
			Type:       "landscape",
			Category:   "image",
			Order:      70,
			Patterns:   []string{`(?i)^landscape(?:[-._ ]?\d+)?$`, `(?i)[-._ ]landscape(?:[-._ ]?\d+)?$`},
			Extensions: imageExtensions,
		},
		{
			ID:         "685ca281-7d66-4807-8d79-acdbaf13aa94",
			Type:       "thumb",
			Category:   "image",
			Order:      80,
			Patterns:   []string{`(?i)^(thumb|thumbnail|characterart|keyart)(?:[-._ ]?\d+)?$`, `(?i)[-._ ]thumb(?:[-._ ]?\d+)?$`, `(?i)^season\d+[-._ ]?thumb$`},
			Extensions: imageExtensions,
		},
		{
			ID:         "e6646d3b-ab5c-4edd-8d9a-064977042a57",
			Type:       "trailer",
			Category:   "video_extra",
			Order:      90,
			Patterns:   []string{`(?i)^trailers?(?:[-._ ]?\d+)?$`, `(?i)[-._ ]trailer(?:[-._ ]?\d+)?$`},
			Extensions: videoExtensions,
		},
		{
			ID:         "b41b9fcf-17fc-4681-a6b0-5034f271f285",
			Type:       "behind_the_scenes",
			Category:   "video_extra",
			Order:      100,
			Patterns:   []string{`(?i)^behind[-._ ]?the[-._ ]?scenes(?:[-._ ]?\d+)?$`, `(?i)[-._ ]behind[-._ ]?the[-._ ]?scenes(?:[-._ ]?\d+)?$`},
			Extensions: videoExtensions,
		},
		{
			ID:         "1459de00-b20f-4566-a8a4-75bc50625d7c",
			Type:       "deleted_scene",
			Category:   "video_extra",
			Order:      110,
			Patterns:   []string{`(?i)^deleted[-._ ]?scenes?(?:[-._ ]?\d+)?$`, `(?i)[-._ ]deleted[-._ ]?scenes?(?:[-._ ]?\d+)?$`},
			Extensions: videoExtensions,
		},
		{
			ID:         "232fff23-0d11-4225-9ec3-f550d285a606",
			Type:       "featurette",
			Category:   "video_extra",
			Order:      120,
			Patterns:   []string{`(?i)^featurettes?(?:[-._ ]?\d+)?$`, `(?i)[-._ ]featurette(?:[-._ ]?\d+)?$`},
			Extensions: videoExtensions,
		},
		{
			ID:         "7576c484-3fd9-42dd-b126-0dc7afb1bf07",
			Type:       "interview",
			Category:   "video_extra",
			Order:      130,
			Patterns:   []string{`(?i)^interviews?(?:[-._ ]?\d+)?$`, `(?i)[-._ ]interview(?:[-._ ]?\d+)?$`},
			Extensions: videoExtensions,
		},
		{
			ID:         "26bde3ff-72eb-4a0d-a566-46d6e7705b8f",
			Type:       "scene",
			Category:   "video_extra",
			Order:      140,
			Patterns:   []string{`(?i)^scenes?(?:[-._ ]?\d+)?$`, `(?i)[-._ ]scene(?:[-._ ]?\d+)?$`},
			Extensions: videoExtensions,
		},
		{
			ID:         "8e276e7a-f85c-4df7-ad97-c90c278a91a3",
			Type:       "short",
			Category:   "video_extra",
			Order:      150,
			Patterns:   []string{`(?i)^shorts?(?:[-._ ]?\d+)?$`, `(?i)[-._ ]short(?:[-._ ]?\d+)?$`},
			Extensions: videoExtensions,
		},
		{
			ID:         "4e644b41-5075-4b67-bbbc-83c334bb4111",
			Type:       "other_extra",
			Category:   "video_extra",
			Order:      160,
			Patterns:   []string{`(?i)^(extras?|other|sample)(?:[-._ ]?\d+)?$`, `(?i)[-._ ](extra|other|sample)(?:[-._ ]?\d+)?$`},
			Extensions: videoExtensions,
		},
		{
			ID:         "c1311474-10c3-4d91-8862-4aed1aca9bae",
			Type:       "theme",
			Category:   "audio",
			Order:      170,
			Patterns:   []string{`(?i)^theme(?:[-._ ]?\d+)?$`, `(?i)[-._ ]theme(?:[-._ ]?\d+)?$`},
			Extensions: audioExtensions,
		},
		{
			ID:         "e905c3d8-2e1e-4d37-b2de-bea29fc0c7b4",
			Type:       "subtitle",
			Category:   "subtitle",
			Order:      180,
			Patterns:   []string{`(?i)^.+$`},
			Extensions: subtitleExtensions,
		},
		{
			ID:         "25325770-1dca-4999-a14d-9100908a1124",
			Type:       "nfo",
			Category:   "metadata",
			Order:      190,
			Patterns:   []string{`(?i)^.+$`},
			Extensions: []string{".nfo"},
		},
		{
			// Disc structure is identified by extension alone: the file names
			// inside a VIDEO_TS or BDMV tree are fixed by the disc format, not
			// by the media.
			ID:         "d76d6f3f-0675-42b5-b849-7ba4ac675508",
			Type:       "disc_structure",
			Category:   "disc_structure",
			Order:      200,
			Patterns:   []string{`(?i)^.+$`},
			Extensions: discExtensions,
		},
		{
			// Trickplay tiles are named 0.jpg, 1.jpg and so on, so nothing about
			// the name identifies them — the scanner names them from their
			// position inside a ".trickplay" folder instead. This pattern is the
			// fallback for a tile found loose, and it is ordered last so every
			// named artwork type gets first refusal on a numeric file name.
			ID:         "9f6b1a2c-58e4-4c7f-9c4a-2f0f6f4b1d3e",
			Type:       "trickplay",
			Category:   "trickplay",
			Order:      210,
			Patterns:   []string{`(?i)^\d+$`},
			Extensions: []string{".jpg"},
		},
	}

	for i := range defaults {
		defaults[i].Patterns = append([]string(nil), defaults[i].Patterns...)
		defaults[i].Extensions = append([]string(nil), defaults[i].Extensions...)
	}
	return defaults
}

// MergeMissingSidecarTypes appends every built-in type the stored table does
// not already carry, returning the merged table and how many were added. It is
// how a type added to the defaults after a database was first seeded still
// reaches that database, since the startup seed only fires on an empty table.
//
// Identity is the entry's id, not its type name, so a built-in someone renamed
// stays renamed rather than being added back under its original name.
//
// The consequence worth knowing: a built-in deleted through the API is added
// back on the next restart, because a deletion leaves nothing behind to
// distinguish it from a type the table has simply never seen.
func MergeMissingSidecarTypes(stored []SidecarTypeDefinition) ([]SidecarTypeDefinition, int) {
	if len(stored) == 0 {
		return stored, 0
	}

	takenIDs := make(map[string]bool, len(stored))
	takenOrders := make(map[int]bool, len(stored))
	highestOrder := 0
	for _, entry := range stored {
		takenIDs[entry.ID] = true
		// Order zero is the disabled sentinel rather than a position, so any
		// number of entries may hold it and it never blocks a slot.
		if entry.Order != 0 {
			takenOrders[entry.Order] = true
			if entry.Order > highestOrder {
				highestOrder = entry.Order
			}
		}
	}

	merged := stored
	added := 0
	for _, missing := range DefaultSidecarTypes() {
		if takenIDs[missing.ID] {
			continue
		}

		// Two enabled entries sharing an order make the table ambiguous, and a
		// table the registry refuses leaves the scanner on its built-in
		// defaults. So a default's own order is used only when that slot is
		// still free in the stored table.
		if takenOrders[missing.Order] {
			highestOrder += 10
			missing.Order = highestOrder
		} else if missing.Order > highestOrder {
			highestOrder = missing.Order
		}
		takenOrders[missing.Order] = true
		takenIDs[missing.ID] = true

		merged = append(merged, missing)
		added++
	}
	return merged, added
}
