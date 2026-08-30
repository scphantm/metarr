package appconfig

import "testing"

const trickplayTypeID = "9f6b1a2c-58e4-4c7f-9c4a-2f0f6f4b1d3e"

// indexOfType returns the position of the entry with the given id, or -1.
func indexOfType(table []SidecarTypeDefinition, id string) int {
	for i, entry := range table {
		if entry.ID == id {
			return i
		}
	}
	return -1
}

// storedTableWithout builds a stored table from the defaults with one entry
// removed, standing in for a database seeded before that entry existed.
func storedTableWithout(id string) []SidecarTypeDefinition {
	defaults := DefaultSidecarTypes()
	stored := make([]SidecarTypeDefinition, 0, len(defaults))
	for _, entry := range defaults {
		if entry.ID != id {
			stored = append(stored, entry)
		}
	}
	return stored
}

func TestMergeMissingSidecarTypesAddsNewBuiltIn(t *testing.T) {
	stored := storedTableWithout(trickplayTypeID)

	merged, added := MergeMissingSidecarTypes(stored)
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}

	index := indexOfType(merged, trickplayTypeID)
	if index < 0 {
		t.Fatal("trickplay type was not added to the merged table")
	}
	if merged[index].Type != "trickplay" || merged[index].Category != "trickplay" {
		t.Errorf("merged entry = %+v, want the trickplay built-in", merged[index])
	}
}

// TestMergeMissingSidecarTypesDoesNotAliasCachedDefaults guards the hazard
// loadBuiltinDefaults's doc comment calls out: a merged-in entry's slices
// must never be the cached parse's own backing arrays, or mutating a merged
// entry in place (as the live config document can be) would corrupt what
// every later Default()/DefaultSidecarTypes() caller sees.
func TestMergeMissingSidecarTypesDoesNotAliasCachedDefaults(t *testing.T) {
	stored := storedTableWithout(trickplayTypeID)

	merged, added := MergeMissingSidecarTypes(stored)
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	index := indexOfType(merged, trickplayTypeID)
	if index < 0 {
		t.Fatal("trickplay type was not added to the merged table")
	}

	// Mutate the merged-in entry's slices in place, the way a live config
	// document's slice can be mutated by later code holding a reference to
	// it, and confirm a fresh read of the built-in defaults is untouched.
	merged[index].Patterns[0] = "corrupted"
	merged[index].Extensions[0] = "corrupted"

	fresh := DefaultSidecarTypes()
	freshIndex := indexOfType(fresh, trickplayTypeID)
	if freshIndex < 0 {
		t.Fatal("trickplay type missing from a fresh DefaultSidecarTypes() read")
	}
	if fresh[freshIndex].Patterns[0] == "corrupted" || fresh[freshIndex].Extensions[0] == "corrupted" {
		t.Fatal("mutating a MergeMissingSidecarTypes result corrupted the cached built-in defaults")
	}
}

func TestMergeMissingSidecarTypesLeavesCompleteTableAlone(t *testing.T) {
	stored := DefaultSidecarTypes()

	merged, added := MergeMissingSidecarTypes(stored)
	if added != 0 {
		t.Errorf("added = %d, want 0 for a table already holding every built-in", added)
	}
	if len(merged) != len(stored) {
		t.Errorf("merged length = %d, want %d", len(merged), len(stored))
	}
}

// TestMergeMissingSidecarTypesSkipsEmptyTable checks the merge defers to the
// startup seed rather than duplicating its job: an empty table means a fresh
// install, which is seeded with the full defaults elsewhere.
func TestMergeMissingSidecarTypesSkipsEmptyTable(t *testing.T) {
	merged, added := MergeMissingSidecarTypes(nil)
	if added != 0 || merged != nil {
		t.Errorf("merge of an empty table = (%v, %d), want (nil, 0)", merged, added)
	}
}

// TestMergeMissingSidecarTypesAvoidsOrderCollision is the important one: two
// enabled entries sharing an order make the table ambiguous, and the registry
// refuses such a table outright, which would drop the scanner back to its
// built-in defaults.
func TestMergeMissingSidecarTypesAvoidsOrderCollision(t *testing.T) {
	stored := storedTableWithout(trickplayTypeID)

	// A user-defined type already sitting in the slot the built-in wants.
	stored = append(stored, SidecarTypeDefinition{
		ID:         "6f1c0f38-0d0e-4a8f-97a9-1c9a5a4e7d21",
		Type:       "user_defined",
		Category:   "image",
		Order:      210,
		Patterns:   []string{`(?i)^custom$`},
		Extensions: []string{".jpg"},
	})

	merged, added := MergeMissingSidecarTypes(stored)
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}

	index := indexOfType(merged, trickplayTypeID)
	if index < 0 {
		t.Fatal("trickplay type was not added to the merged table")
	}
	if merged[index].Order == 210 {
		t.Error("merged entry took order 210, which the stored table already used")
	}

	orders := map[int]string{}
	for _, entry := range merged {
		if entry.Order == 0 {
			continue // the disabled sentinel, which any number of entries may hold
		}
		if existing, taken := orders[entry.Order]; taken {
			t.Errorf("order %d used by both %q and %q", entry.Order, existing, entry.Type)
		}
		orders[entry.Order] = entry.Type
	}
}

// TestMergeMissingSidecarTypesKeepsRenamedBuiltIn covers identity being the id
// rather than the type name: a built-in someone renamed stays renamed instead of
// being added back alongside its original.
func TestMergeMissingSidecarTypesKeepsRenamedBuiltIn(t *testing.T) {
	stored := DefaultSidecarTypes()
	renamed := indexOfType(stored, trickplayTypeID)
	if renamed < 0 {
		t.Fatal("the defaults no longer carry the trickplay type")
	}
	stored[renamed].Type = "previews"

	merged, added := MergeMissingSidecarTypes(stored)
	if added != 0 {
		t.Errorf("added = %d, want 0; the renamed entry still holds the built-in's id", added)
	}
	if merged[renamed].Type != "previews" {
		t.Errorf("renamed entry = %q, want it left as %q", merged[renamed].Type, "previews")
	}
}
