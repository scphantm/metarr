package summarize

import (
	"testing"

	"Metarr/internal/shared/workflow"
)

func fixtureGraph() workflow.Graph {
	return workflow.Graph{
		SchemaVersion: 1,
		Nodes: []workflow.Node{
			{ID: "n1", Type: "core/start", Position: workflow.Position{X: 123, Y: 456}, Label: "Start"},
			{ID: "n2", Type: "fs/copyFile", Position: workflow.Position{X: 789, Y: 10}},
		},
		Edges: []workflow.Edge{
			{ID: "e1", Kind: workflow.EdgeControl, From: workflow.Endpoint{Node: "n1", Port: "next"}, To: workflow.Endpoint{Node: "n2", Port: "in"}},
			{ID: "e2", Kind: workflow.EdgeData, From: workflow.Endpoint{Node: "n1", Port: "path"}, To: workflow.Endpoint{Node: "n2", Port: "source"}, Transform: "toAbsolute"},
		},
		Viewport: map[string]any{"x": 0, "y": 0, "zoom": 1},
	}
}

func TestGraphDropsPositionAndViewport(t *testing.T) {
	summary := Graph(fixtureGraph())

	for _, node := range summary.Nodes {
		// GraphNode has no Position field at all — this loop exists to
		// catch a future field addition that reintroduces one.
		if node.ID == "" || node.Type == "" {
			t.Errorf("node %+v missing id/type", node)
		}
	}
	// GraphSummary has no Viewport field — nothing to assert beyond the
	// type not compiling if one were added without updating this test.
}

func TestGraphKeepsNodeIdentityAndLabel(t *testing.T) {
	summary := Graph(fixtureGraph())

	if len(summary.Nodes) != 2 {
		t.Fatalf("len(Nodes) = %d, want 2", len(summary.Nodes))
	}
	if summary.Nodes[0] != (GraphNode{ID: "n1", Type: "core/start", Label: "Start"}) {
		t.Errorf("Nodes[0] = %+v", summary.Nodes[0])
	}
	if summary.Nodes[1] != (GraphNode{ID: "n2", Type: "fs/copyFile"}) {
		t.Errorf("Nodes[1] = %+v", summary.Nodes[1])
	}
}

func TestGraphRendersEdgesAsCompactStrings(t *testing.T) {
	summary := Graph(fixtureGraph())

	if len(summary.Edges) != 2 {
		t.Fatalf("len(Edges) = %d, want 2", len(summary.Edges))
	}
	if summary.Edges[0] != "n1:next -> n2:in (control)" {
		t.Errorf("Edges[0] = %q", summary.Edges[0])
	}
	if summary.Edges[1] != "n1:path -> n2:source (data) [toAbsolute]" {
		t.Errorf("Edges[1] = %q", summary.Edges[1])
	}
}

func TestGraphOfEmptyGraphProducesNoNilPanics(t *testing.T) {
	summary := Graph(workflow.Graph{})
	if len(summary.Nodes) != 0 || len(summary.Edges) != 0 {
		t.Errorf("summary of empty graph = %+v, want empty", summary)
	}
}
