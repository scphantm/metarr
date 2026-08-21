package summarize

import (
	"fmt"
	"sort"
	"strings"

	"Metarr/internal/shared/workflow"
)

// GraphNode is a trimmed workflow.Node: identity and what it is, not where
// it sits on the canvas.
type GraphNode struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`
}

// GraphSummary is a trimmed workflow.Graph — Position (pure layout,
// meaningless to a model reasoning about wiring) and Viewport are dropped
// entirely; edges are rendered as compact strings rather than nested
// objects, which is both shorter and reads naturally in a prompt.
type GraphSummary struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []string    `json:"edges"`
}

// Graph trims g to its GraphSummary shape. No node-count cap for v1 — worth
// revisiting only if real graphs prove to blow the context budget.
func Graph(g workflow.Graph) GraphSummary {
	nodes := make([]GraphNode, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		nodes = append(nodes, GraphNode{ID: node.ID, Type: node.Type, Label: node.Label})
	}

	edges := make([]string, 0, len(g.Edges))
	for _, edge := range g.Edges {
		edges = append(edges, formatEdge(edge))
	}

	return GraphSummary{Nodes: nodes, Edges: edges}
}

// formatEdge renders "from:port -> to:port (kind)", with a "[transform]"
// suffix when one applies and a "{settings}" suffix when any are set — e.g.
// "n1:out -> n2:in (control)" or "n1:path -> n2:input (data) [toUpper]" or
// "n1:dir -> n2:target (data) {recursive:true}". Settings has no catalog
// schema (see workflow.Edge.Settings), so this stays generic rather than
// naming "recursive" specifically — it renders whatever is set.
func formatEdge(edge workflow.Edge) string {
	rendered := fmt.Sprintf("%s:%s -> %s:%s (%s)", edge.From.Node, edge.From.Port, edge.To.Node, edge.To.Port, edge.Kind)
	if edge.Transform != "" {
		rendered += fmt.Sprintf(" [%s]", edge.Transform)
	}
	if settings := formatSettings(edge.Settings); settings != "" {
		rendered += " " + settings
	}
	return rendered
}

func formatSettings(settings map[string]any) string {
	if len(settings) == 0 {
		return ""
	}
	names := make([]string, 0, len(settings))
	for name := range settings {
		names = append(names, name)
	}
	sort.Strings(names)

	pairs := make([]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, fmt.Sprintf("%s:%v", name, settings[name]))
	}
	return "{" + strings.Join(pairs, ", ") + "}"
}
