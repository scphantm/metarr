package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"Metarr/internal/shared/workflow"
)

// repoCatalogPath is the hand-edited catalog the server ships with.
const repoCatalogPath = "../../../../config/catalog.json"

// TestLoadRealCatalog is the end-to-end check that the shipped catalog.json
// parses into typed entries: the node-kind and effects vocabularies are proto
// enums now, and the file spells them with their enum names.
func TestLoadRealCatalog(t *testing.T) {
	catalog, err := NewLoader(repoCatalogPath).Load()
	if err != nil {
		t.Fatalf("loading %s: %v", repoCatalogPath, err)
	}
	if catalog.Len() == 0 {
		t.Fatal("catalog loaded no entries")
	}

	var sawStart, sawTask, sawDestructive bool
	for _, entry := range catalog.All() {
		switch entry.Kind {
		case workflow.KindStart:
			sawStart = true
		case workflow.KindTask:
			sawTask = true
		}
		if entry.Exec != nil && entry.Exec.Effects == workflow.EffectsDestructive {
			sawDestructive = true
		}
		// Every entry must carry a valid, declared effects classification —
		// the loader would have rejected it otherwise, but assert it here so
		// a regression in enum parsing is obvious.
		if entry.Exec == nil || !workflow.EffectsValid(entry.Exec.Effects) {
			t.Errorf("entry %s (%s) has no valid exec.effects", entry.Type, entry.Id)
		}
	}
	if !sawStart {
		t.Error("no entry parsed as KindStart — enum name parsing may be broken")
	}
	if !sawTask {
		t.Error("no entry parsed as KindTask (the zero value, written by omitting kind)")
	}
	if !sawDestructive {
		t.Error("no entry parsed as EffectsDestructive")
	}
}

// TestAddingANodeTypeIsADataChange confirms a new node type reaches the
// loaded catalog by editing the catalog data alone — no proto change, no
// rebuild of the loader.
func TestAddingANodeTypeIsADataChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	write := func(t *testing.T, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write(t, `{"node_types":[
		{"id":"a","type":"core/start","name":"Start","kind":"WORKFLOW_NODE_KIND_START",
		 "control":{"in":[],"out":["next"]},
		 "exec":{"runs_on":"server","effects":"WORKFLOW_EFFECTS_READ"}}
	]}`)

	loader := NewLoader(path)
	first, err := loader.Load()
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if first.Len() != 1 {
		t.Fatalf("first load: got %d entries, want 1", first.Len())
	}

	// A brand new entry appears simply by being in the file.
	write(t, `{"node_types":[
		{"id":"a","type":"core/start","name":"Start","kind":"WORKFLOW_NODE_KIND_START",
		 "control":{"in":[],"out":["next"]},
		 "exec":{"runs_on":"server","effects":"WORKFLOW_EFFECTS_READ"}},
		{"id":"b","type":"fs/deleteFile","name":"Delete File",
		 "control":{"in":["in"],"out":["next"],"error":true},
		 "exec":{"runs_on":"agent","effects":"WORKFLOW_EFFECTS_DESTRUCTIVE"}}
	]}`)

	second, err := loader.Load()
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if second.Len() != 2 {
		t.Fatalf("second load: got %d entries, want 2", second.Len())
	}
	added, ok := second.Lookup("b")
	if !ok {
		t.Fatal("new entry b not found after a pure data change")
	}
	if added.Kind != workflow.KindTask || added.Exec.Effects != workflow.EffectsDestructive {
		t.Errorf("entry b: kind=%v effects=%v, want task/destructive", added.Kind, added.Exec.Effects)
	}
}

// TestImmutabilityHashSurvivesTheProtoSwitch confirms the published-entry
// immutability check still fires now that entries are hashed with a
// deterministic proto marshal rather than encoding/json.
func TestImmutabilityHashSurvivesTheProtoSwitch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	base := `{"node_types":[
		{"id":"x","type":"core/task","name":"%s",
		 "control":{"in":["in"],"out":["next"]},
		 "exec":{"runs_on":"server","effects":"WORKFLOW_EFFECTS_READ"}}
	]}`

	if err := os.WriteFile(path, []byte(fmt.Sprintf(base, "One")), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := NewLoader(path)
	if _, err := loader.Load(); err != nil {
		t.Fatalf("first load: %v", err)
	}
	// Same load again: unchanged content, no error.
	if _, err := loader.Load(); err != nil {
		t.Fatalf("reload of unchanged catalog: %v", err)
	}
	// Change the entry's content under the same id: must be rejected.
	if err := os.WriteFile(path, []byte(fmt.Sprintf(base, "Two")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(); err == nil {
		t.Fatal("expected a changed published entry to be rejected")
	}
}
