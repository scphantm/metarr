package appconfig

// This file holds the helpers around the sidecar classification table: the
// rules that decide what a non-media file found next to a movie or episode
// actually is. The table lives in configuration rather than in Go so a
// library with unusual naming can be accommodated by editing a document, not
// by editing and redeploying the scanner.
//
// SidecarTypeDefinition itself is an alias to its generated message — see
// model.go. Its field semantics (Patterns are case-insensitive Go regexps
// matched against the extension-stripped base name; Extensions gates the
// match to those lowercase dot-prefixed extensions, empty accepting any;
// Order zero means disabled) are documented on
// proto/metarr/v1/directory_scanner.proto.
//
// The vocabulary the table draws on comes from the Jellyfin and Plex media
// organization specifications:
//
//	https://jellyfin.org/docs/general/server/media/movies
//	https://jellyfin.org/docs/general/server/media/shows/
//	https://support.plex.tv/articles/naming-and-organizing-your-tv-show-files/

import "google.golang.org/protobuf/proto"

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
		defaults[i] = proto.Clone(entry).(*SidecarTypeDefinition)
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
