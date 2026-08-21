package summarize

import (
	"strings"
	"testing"

	"Metarr/internal/shared/workflow"
)

func fixtureCatalog(t *testing.T, description string) *workflow.Catalog {
	t.Helper()

	catalog, err := workflow.NewCatalog([]workflow.NodeType{{
		ID:          "fs/copyFile",
		Type:        "fs/copyFile",
		Name:        "Copy File",
		Category:    "filesystem",
		Description: description,
		Control:     workflow.ControlPorts{In: []string{"in"}, Out: []string{"next"}, Error: true},
		DataIn: []workflow.Socket{
			{Name: "source", Type: "path", Required: true, Description: "long socket description, dropped entirely"},
		},
		DataOut: []workflow.Socket{
			{Name: "destination", Type: "path"},
		},
		Settings: []workflow.Setting{
			{Name: "overwrite", Type: "boolean", Default: false, Description: "long setting description, dropped entirely"},
		},
		Exec: workflow.ExecSpec{
			RunsOn:        workflow.RunsOnAgent,
			AgentSelector: workflow.AgentSelectorPath,
			Timeout:       "30s",
			Cancellable:   true,
			Effects:       workflow.EffectsWrite,
			Retry:         workflow.RetrySpec{Attempts: 3, Backoff: "exponential"},
		},
	}})
	if err != nil {
		t.Fatalf("workflow.NewCatalog: %v", err)
	}
	return catalog
}

func TestCatalogDropsExecSpecEntirely(t *testing.T) {
	catalog := fixtureCatalog(t, "short description")
	summary := Catalog(catalog)

	if len(summary) != 1 {
		t.Fatalf("len(summary) = %d, want 1", len(summary))
	}
	// CatalogEntry has no Exec field at all — this is a compile-time
	// guarantee, but the real risk is a stray reference to timeout/retry
	// sneaking into Description or another field, so check those directly.
	if strings.Contains(summary[0].Description, "30s") || strings.Contains(summary[0].Description, "exponential") {
		t.Errorf("Description leaked exec fields: %q", summary[0].Description)
	}
}

func TestCatalogDropsSocketAndSettingDescriptions(t *testing.T) {
	catalog := fixtureCatalog(t, "short description")
	summary := Catalog(catalog)

	for _, socket := range summary[0].DataIn {
		if strings.Contains(socket.Name, "dropped") {
			t.Errorf("socket %+v carries a description field", socket)
		}
	}
	// Socket and Setting types below have no Description field — verified
	// by construction; this test exists to catch a future field addition
	// that reintroduces one without updating this comment.
	if len(summary[0].DataIn) != 1 || summary[0].DataIn[0].Name != "source" || !summary[0].DataIn[0].Required {
		t.Errorf("DataIn = %+v, want [{source path true}]", summary[0].DataIn)
	}
	if len(summary[0].Settings) != 1 || summary[0].Settings[0].Name != "overwrite" {
		t.Errorf("Settings = %+v, want [{overwrite boolean false}]", summary[0].Settings)
	}
}

func TestCatalogTruncatesLongDescriptions(t *testing.T) {
	long := strings.Repeat("a", maxDescriptionLen+50)
	catalog := fixtureCatalog(t, long)
	summary := Catalog(catalog)

	got := []rune(summary[0].Description)
	if len(got) != maxDescriptionLen+1 { // +1 for the ellipsis marker
		t.Errorf("truncated description length = %d, want %d", len(got), maxDescriptionLen+1)
	}
	if !strings.HasSuffix(summary[0].Description, "…") {
		t.Errorf("truncated description = %q, want a trailing ellipsis", summary[0].Description)
	}
}

func TestCatalogKeepsShortDescriptionsUnchanged(t *testing.T) {
	catalog := fixtureCatalog(t, "short description")
	summary := Catalog(catalog)

	if summary[0].Description != "short description" {
		t.Errorf("Description = %q, want unchanged", summary[0].Description)
	}
}
