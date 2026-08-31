package workflow

import "testing"

func minimalEntry(id, nodeType string) *NodeType {
	return &NodeType{
		Id:      id,
		Type:    nodeType,
		Name:    id,
		Control: &ControlPorts{In: []string{"in"}, Out: []string{"next"}},
		Exec:    &ExecSpec{Effects: EffectsRead},
	}
}

// TestNewCatalogAllowsSharedTypeDistinctID confirms several catalog entries
// may declare the same Type — that's how a plugin offers variations (e.g.
// two core/start entries with different dataOut shapes) without a new
// registered type per variation — as long as their IDs differ.
func TestNewCatalogAllowsSharedTypeDistinctID(t *testing.T) {
	_, err := NewCatalog([]*NodeType{
		minimalEntry("a", "core/start"),
		minimalEntry("b", "core/start"),
	})
	if err != nil {
		t.Fatalf("NewCatalog with distinct ids sharing a Type: %v", err)
	}
}

// TestNewCatalogRejectsDuplicateID confirms id, not type, is the catalog's
// uniqueness key.
func TestNewCatalogRejectsDuplicateID(t *testing.T) {
	_, err := NewCatalog([]*NodeType{
		minimalEntry("dup", "core/start"),
		minimalEntry("dup", "core/end"),
	})
	if err == nil {
		t.Fatal("expected NewCatalog to reject two entries sharing an id")
	}
}

// TestCatalogLookupResolvesByID confirms Lookup finds the exact entry by id,
// not merely a same-typed entry.
func TestCatalogLookupResolvesByID(t *testing.T) {
	catalog, err := NewCatalog([]*NodeType{
		minimalEntry("a", "core/start"),
		minimalEntry("b", "core/start"),
	})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	entry, found := catalog.Lookup("b")
	if !found {
		t.Fatal("Lookup(\"b\") not found")
	}
	if entry.Id != "b" {
		t.Errorf("Lookup(\"b\").Id = %q, want %q", entry.Id, "b")
	}

	if _, found := catalog.Lookup("nonexistent"); found {
		t.Error("Lookup(\"nonexistent\") unexpectedly found")
	}
}

// TestCatalogLookupByTypeFallback confirms the backward-compatibility path
// for graph nodes saved before catalog entries carried an id: a match by
// Type alone, deterministic (catalog-file order) but arbitrary when several
// entries share a Type.
func TestCatalogLookupByTypeFallback(t *testing.T) {
	catalog, err := NewCatalog([]*NodeType{
		minimalEntry("a", "core/start"),
		minimalEntry("b", "core/start"),
	})
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	entry, found := catalog.LookupByType("core/start")
	if !found {
		t.Fatal("LookupByType(\"core/start\") not found")
	}
	if entry.Id != "a" {
		t.Errorf("LookupByType(\"core/start\").Id = %q, want first-in-file %q", entry.Id, "a")
	}

	if _, found := catalog.LookupByType("nonexistent/type"); found {
		t.Error("LookupByType(\"nonexistent/type\") unexpectedly found")
	}
}
