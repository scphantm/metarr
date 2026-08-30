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
	Id string `bson:"id" json:"id"`

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
	Order int32 `bson:"order" json:"order"`

	Patterns   []string `bson:"patterns" json:"patterns"`
	Extensions []string `bson:"extensions" json:"extensions"`
}

// DefaultSidecarTypes returns the built-in classification table, seeded into a
// fresh configuration and restored by the reset endpoint. Callers get their own
// copy: the returned slices are never shared with a previous caller, so a stored
// configuration cannot be mutated through one.
//
// The table itself lives in builtin_defaults.json, not here — see
// docs/adr/0004-bootstrap-module-and-embedded-defaults-file.md for why.
func DefaultSidecarTypes() []*SidecarTypeDefinition {
	return cloneSidecarTypeDefinitions(loadBuiltinDefaults().SidecarTypes)
}

// freshDefaultSidecarTypes is DefaultSidecarTypes without the cached parse
// backing it — see loadBuiltinDefaults's doc comment for why a merge that
// writes stored entries and defaults into the same slice (as
// MergeMissingSidecarTypes does below) must not source from the cached
// singleton.
func freshDefaultSidecarTypes() []*SidecarTypeDefinition {
	return cloneSidecarTypeDefinitions(parseBuiltinDefaults().SidecarTypes)
}

// cloneSidecarTypeDefinitions deep-copies entry, giving the caller a table
// whose Patterns/Extensions slices are never shared with parsed — the
// embedded doc, cached or fresh.
func cloneSidecarTypeDefinitions(parsed []*SidecarTypeDefinition) []*SidecarTypeDefinition {
	defaults := make([]*SidecarTypeDefinition, len(parsed))
	for i, entry := range parsed {
		clone := *entry
		clone.Patterns = append([]string(nil), entry.Patterns...)
		clone.Extensions = append([]string(nil), entry.Extensions...)
		defaults[i] = &clone
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
//
// The defaults merged in come from freshDefaultSidecarTypes, not the cached
// DefaultSidecarTypes — see loadBuiltinDefaults's doc comment.
func MergeMissingSidecarTypes(stored []*SidecarTypeDefinition) ([]*SidecarTypeDefinition, int) {
	if len(stored) == 0 {
		return stored, 0
	}

	takenIDs := make(map[string]bool, len(stored))
	takenOrders := make(map[int32]bool, len(stored))
	highestOrder := int32(0)
	for _, entry := range stored {
		takenIDs[entry.Id] = true
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
	for _, missing := range freshDefaultSidecarTypes() {
		if takenIDs[missing.Id] {
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
		takenIDs[missing.Id] = true

		merged = append(merged, missing)
		added++
	}
	return merged, added
}
