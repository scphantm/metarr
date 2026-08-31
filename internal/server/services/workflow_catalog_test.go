package services

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/handlers"
	"Metarr/internal/shared/workflow"
)

func newTestCatalogServer(t *testing.T) *WorkflowCatalogServer {
	t.Helper()
	catalog, err := workflow.NewCatalog([]*workflow.NodeType{
		{
			Id: "core/start", Type: "core/start", Name: "Start", Kind: workflow.KindStart,
			Control: &workflow.ControlPorts{Out: []string{"next"}},
			Exec:    &workflow.ExecSpec{RunsOn: workflow.RunsOnServer, Effects: workflow.EffectsRead},
		},
		{
			Id: "fs/delete", Type: "fs/delete", Name: "Delete",
			Control: &workflow.ControlPorts{In: []string{"in"}, Out: []string{"next"}, Error: true},
			Exec:    &workflow.ExecSpec{RunsOn: workflow.RunsOnAgent, Effects: workflow.EffectsDestructive},
		},
	})
	if err != nil {
		t.Fatalf("building catalog: %v", err)
	}
	return &WorkflowCatalogServer{Handlers: &handlers.Handlers{
		WorkflowCatalog: catalog,
		Logger:          slog.Default(),
	}}
}

// TestGet_ReturnsTypedCatalog is the behaviour the "catalog is a typed
// message" change is for: Get hands back a WorkflowCatalog message — node
// types with real enum-valued kind/effects, the transform registry, and the
// graph schema version — not an opaque JSON blob.
func TestGet_ReturnsTypedCatalog(t *testing.T) {
	server := newTestCatalogServer(t)

	resp, err := server.Get(context.Background(), connect.NewRequest(&metarrv1.WorkflowCatalogServiceGetRequest{}))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	catalog := resp.Msg.Catalog
	if catalog == nil {
		t.Fatal("Get returned no catalog")
	}
	if len(catalog.NodeTypes) != 2 {
		t.Fatalf("NodeTypes = %d, want 2", len(catalog.NodeTypes))
	}
	if catalog.NodeTypes[0].Kind != workflow.KindStart {
		t.Errorf("NodeTypes[0].Kind = %v, want KindStart", catalog.NodeTypes[0].Kind)
	}
	if catalog.NodeTypes[1].Exec.Effects != workflow.EffectsDestructive {
		t.Errorf("NodeTypes[1].Exec.Effects = %v, want EffectsDestructive", catalog.NodeTypes[1].Exec.Effects)
	}
	if len(catalog.Transforms) == 0 {
		t.Error("Transforms is empty; the registry should be served alongside the catalog")
	}
	if catalog.SchemaVersion != int32(workflow.SchemaVersion) {
		t.Errorf("SchemaVersion = %d, want %d", catalog.SchemaVersion, workflow.SchemaVersion)
	}
}

// TestValidate_ReturnsTypedDiagnostics is the behaviour the "validation
// diagnostics are generated end to end" change is for: Validate hands back
// WorkflowDiagnostic messages with an enum-valued severity — not a
// hand-mapped struct with a "error"/"warning" string — and a graph carrying
// an error-severity diagnostic reports runnable=false.
func TestValidate_ReturnsTypedDiagnostics(t *testing.T) {
	server := newTestCatalogServer(t)

	// A graph with neither a Start nor an End node: two error-severity
	// diagnostics, not runnable.
	graphJSON, err := json.Marshal(map[string]any{
		"schema_version": workflow.SchemaVersion,
		"nodes":          []any{},
		"edges":          []any{},
	})
	if err != nil {
		t.Fatalf("marshalling graph: %v", err)
	}

	resp, err := server.Validate(context.Background(), connect.NewRequest(&metarrv1.WorkflowCatalogServiceValidateRequest{
		GraphJson: graphJSON,
	}))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if resp.Msg.Runnable {
		t.Error("Runnable = true, want false for a graph with error diagnostics")
	}
	if len(resp.Msg.Diagnostics) == 0 {
		t.Fatal("Validate returned no diagnostics for an empty graph")
	}
	for _, d := range resp.Msg.Diagnostics {
		if d.Severity != metarrv1.WorkflowDiagnosticSeverity_WORKFLOW_DIAGNOSTIC_SEVERITY_ERROR {
			t.Errorf("diagnostic %q severity = %v, want ERROR", d.Code, d.Severity)
		}
	}

	codes := make(map[string]bool)
	for _, d := range resp.Msg.Diagnostics {
		codes[d.Code] = true
	}
	if !codes["start.missing"] || !codes["end.missing"] {
		t.Errorf("diagnostics = %v, want start.missing and end.missing", codes)
	}
}

// TestValidate_MalformedGraphIsRejected guards the bad-request path.
func TestValidate_MalformedGraphIsRejected(t *testing.T) {
	server := newTestCatalogServer(t)
	_, err := server.Validate(context.Background(), connect.NewRequest(&metarrv1.WorkflowCatalogServiceValidateRequest{
		GraphJson: []byte("not json"),
	}))
	if err == nil {
		t.Fatal("expected Validate to reject a malformed graph")
	}
}

// TestGet_WithoutACatalogFails guards the not-loaded path.
func TestGet_WithoutACatalogFails(t *testing.T) {
	server := &WorkflowCatalogServer{Handlers: &handlers.Handlers{Logger: slog.Default()}}
	if _, err := server.Get(context.Background(), connect.NewRequest(&metarrv1.WorkflowCatalogServiceGetRequest{})); err == nil {
		t.Fatal("expected Get to fail when no catalog is loaded")
	}
}
