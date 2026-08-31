package workflow

import (
	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// SchemaVersion is the current stored graph format.
//
// Version 0 is the pre-redesign single-edge format, which has no control
// edges at all. Those documents are opened read-only and never auto-migrated:
// the semantics genuinely differ, and a guessed migration produces flows that
// look right and run wrong.
const SchemaVersion = 1

// The graph model. Every type here is an alias to the generated metarr.v1
// message that defines it — proto is the single definition for a model that
// crosses a language boundary, and the graph is authored in the editor,
// validated on the server, and stored in Mongo. See docs/adr/0005.
//
// The node's open content (Settings, Extra) is a *structpb.Struct rather than
// a typed per-node-type message: a node whose Type this build does not
// recognise, and settings it does not recognise, have to survive a
// store-and-load round trip unchanged. The helper methods that hung off Graph
// are package-level functions below, because a method cannot be declared on
// an aliased type.
type (
	Graph    = metarrv1.WorkflowGraph
	Node     = metarrv1.WorkflowGraphNode
	Edge     = metarrv1.WorkflowGraphEdge
	Endpoint = metarrv1.WorkflowEndpoint
	Position = metarrv1.WorkflowGraphPosition
)

// EdgeKind distinguishes the two kinds of wire. They are never styled alike
// in the editor, and they are validated by completely different rules. The
// engine owns the vocabulary and it is closed, so it is a generated enum.
type EdgeKind = metarrv1.WorkflowEdgeKind

const (
	// EdgeKindUnspecified is the zero value; a well-formed edge never carries
	// it.
	EdgeKindUnspecified EdgeKind = metarrv1.WorkflowEdgeKind_WORKFLOW_EDGE_KIND_UNSPECIFIED
	// EdgeControl says what runs next.
	EdgeControl EdgeKind = metarrv1.WorkflowEdgeKind_WORKFLOW_EDGE_KIND_CONTROL
	// EdgeData wires a value into a parameter.
	EdgeData EdgeKind = metarrv1.WorkflowEdgeKind_WORKFLOW_EDGE_KIND_DATA
)

// NodeByID indexes the graph's nodes. A nil graph yields an empty map.
func NodeByID(g *Graph) map[string]*Node {
	if g == nil {
		return map[string]*Node{}
	}
	indexed := make(map[string]*Node, len(g.Nodes))
	for _, node := range g.Nodes {
		indexed[node.Id] = node
	}
	return indexed
}

// ControlEdges returns only the control edges.
func ControlEdges(g *Graph) []*Edge { return edgesOfKind(g, EdgeControl) }

// DataEdges returns only the data edges.
func DataEdges(g *Graph) []*Edge { return edgesOfKind(g, EdgeData) }

func edgesOfKind(g *Graph, kind EdgeKind) []*Edge {
	if g == nil {
		return nil
	}
	var matching []*Edge
	for _, edge := range g.Edges {
		if edge.Kind == kind {
			matching = append(matching, edge)
		}
	}
	return matching
}

// ControlSuccessors maps each node id to the nodes its control edges lead to.
func ControlSuccessors(g *Graph) map[string][]string {
	successors := make(map[string][]string)
	for _, edge := range ControlEdges(g) {
		successors[edge.From.Node] = append(successors[edge.From.Node], edge.To.Node)
	}
	return successors
}

// ControlPredecessors maps each node id to the nodes whose control edges
// lead to it.
func ControlPredecessors(g *Graph) map[string][]string {
	predecessors := make(map[string][]string)
	for _, edge := range ControlEdges(g) {
		predecessors[edge.To.Node] = append(predecessors[edge.To.Node], edge.From.Node)
	}
	return predecessors
}
