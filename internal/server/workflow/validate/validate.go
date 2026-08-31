// Package validate statically checks a drawn workflow before it can be run.
//
// This is where the subtle bugs live, so it is deliberately separable from
// the engine: every rule here is testable against a small hand-built graph
// with no execution, no Mongo, and no agent. See design.md §6.
package validate

import (
	"fmt"
	"sort"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/shared/workflow"
)

// Severity distinguishes what blocks a run from what merely deserves
// attention. Invalid graphs block running, not saving — people save
// half-built flows all the time. It is an alias to the generated metarr.v1
// enum, which is the single definition of the closed vocabulary — see
// docs/adr/0005.
type Severity = metarrv1.WorkflowDiagnosticSeverity

const (
	SeverityError   Severity = metarrv1.WorkflowDiagnosticSeverity_WORKFLOW_DIAGNOSTIC_SEVERITY_ERROR
	SeverityWarning Severity = metarrv1.WorkflowDiagnosticSeverity_WORKFLOW_DIAGNOSTIC_SEVERITY_WARNING
)

// Diagnostic is one finding, shaped so the editor can paint it directly onto
// the canvas. It is an alias to the generated metarr.v1 message: proto is the
// single definition for this model, produced here and rendered by the editor
// with no hand-written mirror on either side (docs/adr/0005).
type Diagnostic = metarrv1.WorkflowDiagnostic

// Result is the outcome of validating one graph.
type Result struct {
	Diagnostics []*Diagnostic `json:"diagnostics"`
}

// Runnable reports whether the graph may be executed.
func (r Result) Runnable() bool {
	for _, diagnostic := range r.Diagnostics {
		if diagnostic.Severity == SeverityError {
			return false
		}
	}
	return true
}

// Graph validates a drawn workflow against the catalog it was authored
// against.
func Graph(graph *workflow.Graph, catalog *workflow.Catalog) Result {
	analysis := newAnalysis(graph, catalog)

	analysis.checkSchemaVersion()
	analysis.checkNodeTypes()
	analysis.checkPortsExist()
	analysis.checkArity()
	analysis.checkStart()
	analysis.checkEnd()
	analysis.checkTerminalPlacement()
	analysis.checkParallelJoins()
	analysis.checkDataEdges()

	sort.SliceStable(analysis.diagnostics, func(i, j int) bool {
		return analysis.diagnostics[i].Code < analysis.diagnostics[j].Code
	})
	return Result{Diagnostics: analysis.diagnostics}
}

type analysis struct {
	graph   *workflow.Graph
	catalog *workflow.Catalog

	nodes map[string]*workflow.Node
	types map[string]*workflow.NodeType // node id -> resolved type

	diagnostics []*Diagnostic

	// mustHaveRun[target] is the set of nodes guaranteed to have executed
	// before target, computed once and reused by every data-edge check.
	mustHaveRun map[string]map[string]bool
	// loopScope[node] is the innermost loop node whose body contains node,
	// or "" when the node is top-level.
	loopScope map[string]string
}

func newAnalysis(graph *workflow.Graph, catalog *workflow.Catalog) *analysis {
	return &analysis{
		graph:   graph,
		catalog: catalog,
		nodes:   workflow.NodeByID(graph),
		types:   make(map[string]*workflow.NodeType),
	}
}

func (a *analysis) report(severity Severity, code, message string, nodeIDs, edgeIDs []string) {
	a.diagnostics = append(a.diagnostics, &Diagnostic{
		Severity: severity, Code: code, Message: message,
		NodeIds: nodeIDs, EdgeIds: edgeIDs,
	})
}

func (a *analysis) checkSchemaVersion() {
	if a.graph.SchemaVersion == workflow.SchemaVersion {
		return
	}
	if a.graph.SchemaVersion < workflow.SchemaVersion {
		a.report(SeverityError, "schema.legacy",
			fmt.Sprintf("This workflow was saved in format %d and predates control and data edges. It opens read-only; it is not migrated automatically because the old and new semantics genuinely differ.", a.graph.SchemaVersion),
			nil, nil)
		return
	}
	a.report(SeverityError, "schema.future",
		fmt.Sprintf("This workflow was saved in format %d by a newer build than this one, which understands up to %d.", a.graph.SchemaVersion, workflow.SchemaVersion),
		nil, nil)
}

// checkNodeTypes resolves every node against the catalog. An unresolved node
// is an error rather than a silent drop — the stored node keeps its settings
// and edges so the flow can be opened, displayed and re-saved without loss.
func (a *analysis) checkNodeTypes() {
	for _, node := range a.graph.Nodes {
		nodeType, found := a.catalog.Lookup(node.CatalogId)
		if !found && node.CatalogId == "" {
			// Legacy save from before catalog entries carried an id — fall
			// back to an arbitrary but deterministic match by Type.
			nodeType, found = a.catalog.LookupByType(node.Type)
		}
		if !found {
			a.report(SeverityError, "type.missing",
				fmt.Sprintf("No node type %s is installed, so this node cannot run. Its settings and connections are preserved.", node.Type),
				[]string{node.Id}, nil)
			continue
		}
		a.types[node.Id] = nodeType
	}
}

// executable reports whether a node participates in control flow at all.
// Notes are annotations and pure sources are not execution steps, so both are
// excluded from every control-flow rule.
func (a *analysis) executable(nodeID string) bool {
	nodeType, resolved := a.types[nodeID]
	if !resolved {
		return false
	}
	return nodeType.Kind != workflow.KindNote && nodeType.Kind != workflow.KindSource
}

func (a *analysis) checkPortsExist() {
	for _, edge := range a.graph.Edges {
		fromType, fromResolved := a.types[edge.From.Node]
		toType, toResolved := a.types[edge.To.Node]
		if !fromResolved || !toResolved {
			continue // already reported as an unresolved type
		}

		switch edge.Kind {
		case workflow.EdgeControl:
			if !workflow.HasControlOut(fromType, edge.From.Port) {
				a.report(SeverityError, "port.unknownControlOut",
					fmt.Sprintf("%s has no control output named %q.", fromType.Name, edge.From.Port),
					[]string{edge.From.Node}, []string{edge.Id})
			}
			if !workflow.HasControlIn(toType, edge.To.Port) {
				a.report(SeverityError, "port.unknownControlIn",
					fmt.Sprintf("%s has no control input named %q.", toType.Name, edge.To.Port),
					[]string{edge.To.Node}, []string{edge.Id})
			}
		case workflow.EdgeData:
			if _, found := workflow.DataOutSocket(fromType, edge.From.Port); !found && !a.isInferredOutput(fromType, edge.From.Port) {
				a.report(SeverityError, "port.unknownDataOut",
					fmt.Sprintf("%s has no data output named %q.", fromType.Name, edge.From.Port),
					[]string{edge.From.Node}, []string{edge.Id})
			}
			if _, found := workflow.DataInSocket(toType, edge.To.Port); !found {
				a.report(SeverityError, "port.unknownDataIn",
					fmt.Sprintf("%s has no data input named %q.", toType.Name, edge.To.Port),
					[]string{edge.To.Node}, []string{edge.Id})
			}
		default:
			a.report(SeverityError, "edge.unknownKind",
				fmt.Sprintf("Edge %s has kind %s; expected control or data.", edge.Id, edge.Kind),
				nil, []string{edge.Id})
		}
	}
}

// isInferredOutput covers the two ports whose declared type is a placeholder
// resolved from what is wired in — see resolveOutputType.
func (a *analysis) isInferredOutput(nodeType *workflow.NodeType, port string) bool {
	switch nodeType.Kind {
	case workflow.KindForEach:
		return port == "item"
	case workflow.KindCollect:
		return port == "collected"
	}
	return false
}

// checkArity enforces the cardinality rules that make the rest of the
// analysis decidable. A control out-port taking exactly one edge is what
// forbids implicit fan-out, which is what makes join arity static and
// deadlock structurally impossible.
func (a *analysis) checkArity() {
	controlOutUse := map[string][]string{}
	dataInUse := map[string][]string{}

	for _, edge := range a.graph.Edges {
		switch edge.Kind {
		case workflow.EdgeControl:
			key := edge.From.Node + "\x00" + edge.From.Port
			controlOutUse[key] = append(controlOutUse[key], edge.Id)
		case workflow.EdgeData:
			key := edge.To.Node + "\x00" + edge.To.Port
			dataInUse[key] = append(dataInUse[key], edge.Id)
		}
	}

	for key, edgeIDs := range controlOutUse {
		if len(edgeIDs) <= 1 {
			continue
		}
		nodeID, port := splitKey(key)
		sort.Strings(edgeIDs)
		a.report(SeverityError, "arity.controlOut",
			fmt.Sprintf("%q already leads somewhere. To run two things at once, insert a Parallel node — a second wire here would be an accidental race, not a fork.", port),
			[]string{nodeID}, edgeIDs)
	}

	for key, edgeIDs := range dataInUse {
		if len(edgeIDs) <= 1 {
			continue
		}
		nodeID, port := splitKey(key)
		sort.Strings(edgeIDs)
		a.report(SeverityError, "arity.dataIn",
			fmt.Sprintf("Input %q is wired from more than one source, so which value it receives is ambiguous.", port),
			[]string{nodeID}, edgeIDs)
	}
}

func splitKey(key string) (nodeID, port string) {
	for index := 0; index < len(key); index++ {
		if key[index] == 0 {
			return key[:index], key[index+1:]
		}
	}
	return key, ""
}

func (a *analysis) checkStart() {
	var startNodes []string
	for _, node := range a.graph.Nodes {
		if nodeType, resolved := a.types[node.Id]; resolved && nodeType.Kind == workflow.KindStart {
			startNodes = append(startNodes, node.Id)
		}
	}

	switch len(startNodes) {
	case 1:
	case 0:
		a.report(SeverityError, "start.missing",
			"This workflow has no Start node, so there is nowhere for a run to begin.", nil, nil)
	default:
		sort.Strings(startNodes)
		a.report(SeverityError, "start.multiple",
			"A workflow may only have one Start node.", startNodes, nil)
	}
}

// checkEnd requires at least one End node. Unlike Start, more than one is
// fine — different branches ending the run at different points is normal —
// so this only ever fires on zero, never on "multiple".
func (a *analysis) checkEnd() {
	for _, node := range a.graph.Nodes {
		if nodeType, resolved := a.types[node.Id]; resolved && nodeType.Kind == workflow.KindEnd {
			return
		}
	}
	a.report(SeverityError, "end.missing",
		"This workflow has no End node, so a run has nowhere to finish.", nil, nil)
}

// startNode returns the single start node id, or "" when there is not
// exactly one.
func (a *analysis) startNode() string {
	var found string
	for _, node := range a.graph.Nodes {
		if nodeType, resolved := a.types[node.Id]; resolved && nodeType.Kind == workflow.KindStart {
			if found != "" {
				return ""
			}
			found = node.Id
		}
	}
	return found
}
