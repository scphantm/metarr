package pagecontext

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"Metarr/internal/shared/workflow"
)

func fixtureWorkflowCatalog(t *testing.T) *workflow.Catalog {
	t.Helper()

	catalog, err := workflow.NewCatalog([]workflow.NodeType{{
		ID:          "fs/copyFile",
		Type:        "fs/copyFile",
		Name:        "Copy File",
		Control:     workflow.ControlPorts{In: []string{"in"}, Out: []string{"next"}},
		Exec:        workflow.ExecSpec{Effects: workflow.EffectsWrite},
	}})
	if err != nil {
		t.Fatalf("workflow.NewCatalog: %v", err)
	}
	return catalog
}

func TestWorkflowAssemblerPageKey(t *testing.T) {
	assembler := NewWorkflowAssembler(fixtureWorkflowCatalog(t))
	if assembler.PageKey() != "workflow" {
		t.Errorf("PageKey() = %q, want %q", assembler.PageKey(), "workflow")
	}
}

func TestWorkflowAssemblerRejectsInvalidPayload(t *testing.T) {
	assembler := NewWorkflowAssembler(fixtureWorkflowCatalog(t))
	if _, err := assembler.Assemble(context.Background(), json.RawMessage(`not json`)); err == nil {
		t.Fatal("Assemble accepted malformed JSON")
	}
}

func TestWorkflowAssemblerWithNoGraphOnlySendsCatalog(t *testing.T) {
	assembler := NewWorkflowAssembler(fixtureWorkflowCatalog(t))

	assembled, err := assembler.Assemble(context.Background(), json.RawMessage(`{"documentId":"","graph":null}`))
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	if len(assembled.Sent.Items) != 1 {
		t.Fatalf("Sent.Items = %+v, want exactly the catalog item", assembled.Sent.Items)
	}
	if assembled.Sent.Items[0].Label != "Node catalog" {
		t.Errorf("Items[0].Label = %q, want %q", assembled.Sent.Items[0].Label, "Node catalog")
	}
	if !strings.Contains(assembled.SystemText, "fs/copyFile") {
		t.Errorf("SystemText missing the catalog entry: %q", assembled.SystemText)
	}
}

func TestWorkflowAssemblerWithGraphSendsBothItems(t *testing.T) {
	assembler := NewWorkflowAssembler(fixtureWorkflowCatalog(t))

	payload := `{
		"documentId": "abc123",
		"meta": {"name": "My Workflow"},
		"graph": {
			"schema_version": 1,
			"nodes": [{"id": "n1", "type": "fs/copyFile", "catalogId": "fs/copyFile"}],
			"edges": []
		}
	}`

	assembled, err := assembler.Assemble(context.Background(), json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	if len(assembled.Sent.Items) != 2 {
		t.Fatalf("Sent.Items = %+v, want catalog + graph", assembled.Sent.Items)
	}
	if assembled.Sent.Items[1].Label != "Current workflow" {
		t.Errorf("Items[1].Label = %q, want %q", assembled.Sent.Items[1].Label, "Current workflow")
	}
	if !strings.Contains(assembled.SystemText, "My Workflow") {
		t.Errorf("SystemText missing the workflow name: %q", assembled.SystemText)
	}
	if !strings.Contains(assembled.SystemText, `"n1"`) {
		t.Errorf("SystemText missing the graph's node: %q", assembled.SystemText)
	}
}

// Gating propose_workflow_edit to the workflow assembler is what makes "the
// AI can only propose edits on the workflow page" a structural fact, not a
// prompt instruction that could be talked around — this test is the
// guarantee for that half of the contract.
func TestWorkflowAssemblerContributesProposeEditTool(t *testing.T) {
	assembler := NewWorkflowAssembler(fixtureWorkflowCatalog(t))
	tools := assembler.Tools()

	if len(tools) != 1 || tools[0].Name != proposeWorkflowEditToolName {
		t.Fatalf("Tools() = %+v, want exactly [%s]", tools, proposeWorkflowEditToolName)
	}
	var schema map[string]any
	if err := json.Unmarshal(tools[0].JSONSchema, &schema); err != nil {
		t.Errorf("Tools()[0].JSONSchema is not valid JSON: %v", err)
	}
}
