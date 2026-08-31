package services

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/mongostore"
)

func mustStruct(t *testing.T, fields map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(fields)
	if err != nil {
		t.Fatalf("building struct: %v", err)
	}
	return s
}

// TestWorkflowGraphSurvivesStoreAndLoad is the single most important test in
// the model-generation work: the guard against silently destroying a user's
// workflow.
//
// A node whose type this build does not recognise, and settings it does not
// recognise, must round-trip through the workflow service's store-and-load
// conversion unchanged. The graph carries its open content as structured
// values (settings and extra) precisely so a typed schema cannot drop it —
// the design's passthrough promise, asserted here as external behaviour at
// the service seam rather than left as a comment warning someone not to
// break it.
func TestWorkflowGraphSurvivesStoreAndLoad(t *testing.T) {
	original := &metarrv1.WorkflowGraph{
		SchemaVersion: 1,
		Nodes: []*metarrv1.WorkflowGraphNode{
			{
				Id:        "n1",
				Type:      "vendor/node-this-build-has-never-heard-of",
				CatalogId: "vendor/node@9.9.9",
				Position:  &metarrv1.WorkflowGraphPosition{X: 12.5, Y: -40},
				Settings: mustStruct(t, map[string]any{
					"knownLooking":  "value",
					"unrecognised":  42.0,
					"deeplyUnknown": map[string]any{"keep": true, "list": []any{1.0, 2.0}},
				}),
				Promoted: []string{"knownLooking"},
				Label:    "keep my label",
				Extra: mustStruct(t, map[string]any{
					"shapeColor":  "violet",
					"borderColor": "cyan",
					"futureField": "from a newer build",
				}),
			},
			{
				Id:       "n2",
				Type:     "core/end",
				Position: &metarrv1.WorkflowGraphPosition{X: 0, Y: 0},
			},
		},
		Edges: []*metarrv1.WorkflowGraphEdge{
			{
				Id:   "e1",
				Kind: metarrv1.WorkflowEdgeKind_WORKFLOW_EDGE_KIND_CONTROL,
				From: &metarrv1.WorkflowEndpoint{Node: "n1", Port: "next"},
				To:   &metarrv1.WorkflowEndpoint{Node: "n2", Port: "in"},
			},
			{
				Id:        "e2",
				Kind:      metarrv1.WorkflowEdgeKind_WORKFLOW_EDGE_KIND_DATA,
				From:      &metarrv1.WorkflowEndpoint{Node: "n1", Port: "out"},
				To:        &metarrv1.WorkflowEndpoint{Node: "n2", Port: "value"},
				Transform: "parentDir",
				Settings:  mustStruct(t, map[string]any{"recursive": true}),
			},
		},
		Viewport: mustStruct(t, map[string]any{"x": 1.0, "y": 2.0, "zoom": 1.5}),
	}

	req := &metarrv1.WorkflowServiceUpsertRequest{
		Name: "keeps unknown things", Description: "d", Tags: []string{"t"}, Graph: original,
	}

	// Store: the request becomes the loose document mongostore persists.
	stored, err := workflowFromUpsertProto(req)
	if err != nil {
		t.Fatalf("workflowFromUpsertProto: %v", err)
	}

	// Simulate the Mongo round trip the versioned store performs — BSON
	// encode and decode of the whole document — so a type coercion in the
	// driver is caught here rather than in production.
	raw, err := bson.Marshal(stored)
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}
	var reloaded mongostore.Workflow
	if err := bson.Unmarshal(raw, &reloaded); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	// Load: the stored document becomes the graph message again.
	loaded, err := graphToProto(reloaded)
	if err != nil {
		t.Fatalf("graphToProto: %v", err)
	}

	if !proto.Equal(original, loaded) {
		t.Fatalf("graph did not survive store-and-load:\n before: %v\n after:  %v", original, loaded)
	}
}

// TestWorkflowUpsertAcceptsAnEmptyGraph pins that the retype did not make a
// blank canvas un-saveable — people save half-built flows all the time.
func TestWorkflowUpsertAcceptsAnEmptyGraph(t *testing.T) {
	// An empty graph is legitimate — people save blank canvases — so the
	// conversion must not reject it.
	stored, err := workflowFromUpsertProto(&metarrv1.WorkflowServiceUpsertRequest{
		Name: "n", Description: "d", Tags: []string{"t"},
		Graph: &metarrv1.WorkflowGraph{SchemaVersion: 1},
	})
	if err != nil {
		t.Fatalf("workflowFromUpsertProto on an empty graph: %v", err)
	}
	if stored.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", stored.SchemaVersion)
	}
}
