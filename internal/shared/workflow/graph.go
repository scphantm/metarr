package workflow

import "encoding/json"

// SchemaVersion is the current stored graph format.
//
// Version 0 is the pre-redesign single-edge format, which has no control
// edges at all. Those documents are opened read-only and never auto-migrated:
// the semantics genuinely differ, and a guessed migration produces flows that
// look right and run wrong.
const SchemaVersion = 1

// Position is a node's place on the canvas. Purely presentational.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Node is one placed instance of a catalog type.
type Node struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	TypeVersion string   `json:"typeVersion"`
	Position    Position `json:"position"`
	// Settings holds the literal values the user entered, keyed by the
	// catalog's setting names.
	Settings map[string]any `json:"settings,omitempty"`
	// Promoted lists settings turned into wired data-in sockets on this
	// instance.
	Promoted []string `json:"promoted,omitempty"`
	// Label overrides the catalog's display name for this instance.
	Label string `json:"label,omitempty"`

	// Extra preserves any field this version of the schema does not
	// recognise, so that loading and re-saving a workflow written by a newer
	// build does not quietly delete parts of it.
	//
	// This matters more than it looks. The previous storage kept nodes as
	// opaque documents and therefore preserved everything by accident; a
	// typed struct silently drops whatever it has no field for, which is the
	// easiest way in this whole design to destroy a user's work.
	Extra map[string]json.RawMessage `json:"-"`
}

// knownNodeFields are the keys Node models directly; everything else in the
// incoming object is preserved in Extra.
var knownNodeFields = map[string]bool{
	"id": true, "type": true, "typeVersion": true, "position": true,
	"settings": true, "promoted": true, "label": true,
}

// nodeFields mirrors Node without the custom marshalling, so the encoder can
// be reused without recursing.
type nodeFields struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	TypeVersion string         `json:"typeVersion"`
	Position    Position       `json:"position"`
	Settings    map[string]any `json:"settings,omitempty"`
	Promoted    []string       `json:"promoted,omitempty"`
	Label       string         `json:"label,omitempty"`
}

// UnmarshalJSON decodes a node, routing unrecognised keys into Extra.
func (n *Node) UnmarshalJSON(data []byte) error {
	var fields nodeFields
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*n = Node{
		ID:          fields.ID,
		Type:        fields.Type,
		TypeVersion: fields.TypeVersion,
		Position:    fields.Position,
		Settings:    fields.Settings,
		Promoted:    fields.Promoted,
		Label:       fields.Label,
	}
	for key, value := range raw {
		if knownNodeFields[key] {
			continue
		}
		if n.Extra == nil {
			n.Extra = make(map[string]json.RawMessage)
		}
		n.Extra[key] = value
	}
	return nil
}

// MarshalJSON re-emits the node with its preserved unknown fields.
func (n Node) MarshalJSON() ([]byte, error) {
	encoded, err := json.Marshal(nodeFields{
		ID:          n.ID,
		Type:        n.Type,
		TypeVersion: n.TypeVersion,
		Position:    n.Position,
		Settings:    n.Settings,
		Promoted:    n.Promoted,
		Label:       n.Label,
	})
	if err != nil {
		return nil, err
	}
	if len(n.Extra) == 0 {
		return encoded, nil
	}

	var merged map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &merged); err != nil {
		return nil, err
	}
	for key, value := range n.Extra {
		// Known fields win: a stale Extra key must never shadow a value the
		// current schema actually models.
		if _, isKnown := merged[key]; isKnown {
			continue
		}
		merged[key] = value
	}
	return json.Marshal(merged)
}

// EdgeKind distinguishes the two kinds of wire. They are never styled alike
// in the editor, and they are validated by completely different rules.
type EdgeKind string

const (
	// EdgeControl says what runs next.
	EdgeControl EdgeKind = "control"
	// EdgeData wires a value into a parameter.
	EdgeData EdgeKind = "data"
)

// Endpoint is one end of an edge: a node and one of its ports.
type Endpoint struct {
	Node string `json:"node"`
	Port string `json:"port"`
}

// Edge connects two ports.
type Edge struct {
	ID   string   `json:"id"`
	Kind EdgeKind `json:"kind"`
	From Endpoint `json:"from"`
	To   Endpoint `json:"to"`
	// Transform names an explicit conversion applied to the value in flight.
	// Only meaningful on data edges, and always a single registered name
	// rather than a chain.
	Transform string `json:"transform,omitempty"`
}

// Graph is a stored workflow's node and edge content.
type Graph struct {
	SchemaVersion int            `json:"schema_version"`
	Nodes         []Node         `json:"nodes"`
	Edges         []Edge         `json:"edges"`
	Viewport      map[string]any `json:"viewport,omitempty"`
}

// NodeByID indexes the graph's nodes.
func (g Graph) NodeByID() map[string]Node {
	indexed := make(map[string]Node, len(g.Nodes))
	for _, node := range g.Nodes {
		indexed[node.ID] = node
	}
	return indexed
}

// ControlEdges returns only the control edges.
func (g Graph) ControlEdges() []Edge { return g.edgesOfKind(EdgeControl) }

// DataEdges returns only the data edges.
func (g Graph) DataEdges() []Edge { return g.edgesOfKind(EdgeData) }

func (g Graph) edgesOfKind(kind EdgeKind) []Edge {
	var matching []Edge
	for _, edge := range g.Edges {
		if edge.Kind == kind {
			matching = append(matching, edge)
		}
	}
	return matching
}

// ControlSuccessors maps each node id to the nodes its control edges lead to.
func (g Graph) ControlSuccessors() map[string][]string {
	successors := make(map[string][]string, len(g.Nodes))
	for _, edge := range g.ControlEdges() {
		successors[edge.From.Node] = append(successors[edge.From.Node], edge.To.Node)
	}
	return successors
}

// ControlPredecessors maps each node id to the nodes whose control edges
// lead to it.
func (g Graph) ControlPredecessors() map[string][]string {
	predecessors := make(map[string][]string, len(g.Nodes))
	for _, edge := range g.ControlEdges() {
		predecessors[edge.To.Node] = append(predecessors[edge.To.Node], edge.From.Node)
	}
	return predecessors
}
