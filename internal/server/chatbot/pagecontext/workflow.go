package pagecontext

import (
	"context"
	"encoding/json"
	"fmt"

	"Metarr/internal/server/chatbot/provider"
	"Metarr/internal/server/chatbot/summarize"
	"Metarr/internal/shared/workflow"
)

// proposeWorkflowEditToolName is the tool name the model calls to propose a
// graph change. Only the workflow assembler contributes it (see Tools
// below), so it's never even offered to the model on any other page.
const proposeWorkflowEditToolName = "propose_workflow_edit"

// proposeWorkflowEditSchema describes { graph: workflow.Graph, summary:
// string } — summary is the model's plain-English explanation, shown to the
// user before they approve anything (see internal/server/chatbot's
// permission-gated edit flow).
const proposeWorkflowEditSchema = `{
  "type": "object",
  "required": ["graph", "summary"],
  "properties": {
    "summary": {
      "type": "string",
      "description": "A short, plain-English explanation of what this edit does, shown to the user before they approve it."
    },
    "graph": {
      "type": "object",
      "required": ["schema_version", "nodes", "edges"],
      "properties": {
        "schema_version": { "type": "integer" },
        "nodes": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["id", "type", "catalogId"],
            "properties": {
              "id": { "type": "string" },
              "type": { "type": "string", "description": "A catalog type, e.g. fs/copyFile." },
              "catalogId": { "type": "string", "description": "The exact catalog entry's id (from the node catalog above). Several entries may share a type — this is what picks the right one, e.g. between two core/start variants with different dataOut shapes." },
              "position": {
                "type": "object",
                "properties": { "x": { "type": "number" }, "y": { "type": "number" } }
              },
              "settings": { "type": "object" },
              "label": { "type": "string" }
            }
          }
        },
        "edges": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["id", "kind", "from", "to"],
            "properties": {
              "id": { "type": "string" },
              "kind": { "type": "string", "enum": ["control", "data"] },
              "from": {
                "type": "object",
                "required": ["node", "port"],
                "properties": { "node": { "type": "string" }, "port": { "type": "string" } }
              },
              "to": {
                "type": "object",
                "required": ["node", "port"],
                "properties": { "node": { "type": "string" }, "port": { "type": "string" } }
              },
              "transform": { "type": "string" },
              "settings": { "type": "object", "description": "Per-edge configuration, e.g. { \"recursive\": true } on a data edge delivering a path." }
            }
          }
        }
      }
    }
  }
}`

// WorkflowAssembler is the only concrete Assembler wired up in this first
// pass — the Search page (and any other) plugs in the same way later with
// no changes to Registry, the chat widget, or this package's contract.
type WorkflowAssembler struct {
	catalog *workflow.Catalog
}

// NewWorkflowAssembler constructs the workflow page's Assembler. catalog is
// the same *workflow.Catalog the workflow HTTP handlers already hold —
// building context from it never needs its own HTTP round-trip.
func NewWorkflowAssembler(catalog *workflow.Catalog) *WorkflowAssembler {
	return &WorkflowAssembler{catalog: catalog}
}

// PageKey implements Assembler.
func (w *WorkflowAssembler) PageKey() string { return "workflow" }

// workflowClientPayload mirrors what WorkflowEditorPage's
// useRegisterPageContext collect() function sends — see
// ui/src/pages/workflows/WorkflowEditorPage.tsx.
type workflowClientPayload struct {
	DocumentID string `json:"documentId"`
	Meta       struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	} `json:"meta"`
	// Graph is nil when the canvas has no ReactFlow instance yet (e.g. the
	// page just mounted) — the live canvas may hold unsaved edits Mongo
	// doesn't have, so this always comes from the client rather than being
	// re-read from storage server-side.
	Graph *workflow.Graph `json:"graph"`
}

// Assemble implements Assembler.
func (w *WorkflowAssembler) Assemble(_ context.Context, clientPayload json.RawMessage) (Assembled, error) {
	var payload workflowClientPayload
	if err := json.Unmarshal(clientPayload, &payload); err != nil {
		return Assembled{}, fmt.Errorf("pagecontext: invalid workflow context payload: %w", err)
	}

	catalogSummary := summarize.Catalog(w.catalog)
	catalogJSON, err := json.Marshal(catalogSummary)
	if err != nil {
		return Assembled{}, fmt.Errorf("pagecontext: marshal catalog summary: %w", err)
	}

	items := []ContextSentItem{{
		Label:         "Node catalog",
		Description:   fmt.Sprintf("%d node types, ports and settings only", len(catalogSummary)),
		TokenEstimate: estimateTokens(string(catalogJSON)),
		Detail:        catalogJSON,
	}}

	systemText := "You are helping edit a Metarr workflow. Here is the available node catalog " +
		"(each entry's control/data ports and settings — this is everything you may reference " +
		"by type name):\n" + string(catalogJSON)

	if payload.Graph != nil {
		graphSummary := summarize.Graph(*payload.Graph)
		graphJSON, err := json.Marshal(graphSummary)
		if err != nil {
			return Assembled{}, fmt.Errorf("pagecontext: marshal graph summary: %w", err)
		}

		items = append(items, ContextSentItem{
			Label:         "Current workflow",
			Description:   fmt.Sprintf("%d nodes, %d edges", len(graphSummary.Nodes), len(graphSummary.Edges)),
			TokenEstimate: estimateTokens(string(graphJSON)),
			Detail:        graphJSON,
		})

		systemText += "\n\nThe workflow currently open (name: " + payload.Meta.Name + "):\n" + string(graphJSON)
	}

	return Assembled{
		SystemText: systemText,
		Sent:       ContextSentRecord{PageKey: w.PageKey(), Items: items},
	}, nil
}

// Tools implements Assembler. Gating propose_workflow_edit to only this
// assembler is what makes "the AI can only propose edits on the workflow
// page" a structural fact, not a prompt instruction.
func (w *WorkflowAssembler) Tools() []provider.ToolSpec {
	return []provider.ToolSpec{{
		Name:        proposeWorkflowEditToolName,
		Description: "Propose a change to the currently-open workflow graph. The user reviews and must explicitly approve it before anything is applied.",
		JSONSchema:  json.RawMessage(proposeWorkflowEditSchema),
	}}
}
