package validate_test

import (
	"strings"
	"testing"

	"Metarr/internal/server/workflow/validate"
	"Metarr/internal/shared/workflow"
)

// testCatalog builds a minimal catalog covering every node kind the
// analysis special-cases, so the graph fixtures below stay readable.
func testCatalog(t *testing.T) *workflow.Catalog {
	t.Helper()

	readOnServer := workflow.ExecSpec{RunsOn: workflow.RunsOnServer, Effects: workflow.EffectsRead}
	branchPorts := []string{"branch1", "branch2", "branch3", "branch4"}

	catalog, err := workflow.NewCatalog([]workflow.NodeType{
		{
			Type: "t/start", TypeVersion: "1", Name: "Start", Kind: workflow.KindStart,
			Control: workflow.ControlPorts{Out: []string{"next"}},
			Exec:    readOnServer,
		},
		{
			Type: "t/task", TypeVersion: "1", Name: "Task",
			Control: workflow.ControlPorts{In: []string{"in"}, Out: []string{"next"}, Error: true},
			DataIn:  []workflow.Socket{{Name: "value", Type: workflow.TypeAny}},
			DataOut: []workflow.Socket{{Name: "result", Type: workflow.TypePathFile}},
			Exec:    readOnServer,
		},
		{
			Type: "t/dirTask", TypeVersion: "1", Name: "Directory Task",
			Control: workflow.ControlPorts{In: []string{"in"}, Out: []string{"next"}},
			DataIn:  []workflow.Socket{{Name: "folder", Type: workflow.TypePathDir}},
			Exec:    readOnServer,
		},
		{
			Type: "t/branch", TypeVersion: "1", Name: "Branch", Kind: workflow.KindBranch,
			Control: workflow.ControlPorts{In: []string{"in"}, Out: []string{"yes", "no"}},
			Exec:    readOnServer,
		},
		{
			Type: "t/forEach", TypeVersion: "1", Name: "For Each", Kind: workflow.KindForEach,
			Control: workflow.ControlPorts{In: []string{"in"}, Out: []string{"body", "done"}},
			DataIn:  []workflow.Socket{{Name: "collection", Type: workflow.ListOf(workflow.TypeAny), Required: true}},
			DataOut: []workflow.Socket{
				{Name: "item", Type: workflow.TypeAny},
				{Name: "index", Type: workflow.TypeNumberInt},
			},
			Exec: readOnServer,
		},
		{
			Type: "t/collect", TypeVersion: "1", Name: "Collect", Kind: workflow.KindCollect,
			Control: workflow.ControlPorts{In: []string{"in"}, Out: []string{"next"}},
			DataIn:  []workflow.Socket{{Name: "value", Type: workflow.TypeAny, Required: true}},
			DataOut: []workflow.Socket{{Name: "collected", Type: workflow.ListOf(workflow.TypeAny)}},
			Exec:    readOnServer,
		},
		{
			Type: "t/parallel", TypeVersion: "1", Name: "Parallel", Kind: workflow.KindParallel,
			Control:  workflow.ControlPorts{In: []string{"in"}, Out: branchPorts},
			Settings: []workflow.Setting{{Name: "branches", Type: workflow.TypeNumberInt, Default: 2}},
			Exec:     readOnServer,
		},
		{
			Type: "t/join", TypeVersion: "1", Name: "Join", Kind: workflow.KindJoin,
			Control: workflow.ControlPorts{In: branchPorts, Out: []string{"next"}},
			Exec:    readOnServer,
		},
		{
			Type: "t/end", TypeVersion: "1", Name: "End", Kind: workflow.KindEnd,
			Control: workflow.ControlPorts{In: []string{"in"}},
			Exec:    readOnServer,
		},
		{
			Type: "t/source", TypeVersion: "1", Name: "Literal Path", Kind: workflow.KindSource,
			Control: workflow.ControlPorts{},
			DataOut: []workflow.Socket{{Name: "path", Type: workflow.TypePathFile}},
			Exec:    readOnServer,
		},
		{
			Type: "t/listSource", TypeVersion: "1", Name: "File List", Kind: workflow.KindSource,
			Control: workflow.ControlPorts{},
			DataOut: []workflow.Socket{{Name: "files", Type: workflow.ListOf(workflow.TypePathFile)}},
			Exec:    readOnServer,
		},
	})
	if err != nil {
		t.Fatalf("building test catalog: %v", err)
	}
	return catalog
}

func node(id, nodeType string) workflow.Node {
	return workflow.Node{ID: id, Type: nodeType, TypeVersion: "1"}
}

func control(id, fromNode, fromPort, toNode, toPort string) workflow.Edge {
	return workflow.Edge{
		ID: id, Kind: workflow.EdgeControl,
		From: workflow.Endpoint{Node: fromNode, Port: fromPort},
		To:   workflow.Endpoint{Node: toNode, Port: toPort},
	}
}

func data(id, fromNode, fromPort, toNode, toPort string) workflow.Edge {
	return workflow.Edge{
		ID: id, Kind: workflow.EdgeData,
		From: workflow.Endpoint{Node: fromNode, Port: fromPort},
		To:   workflow.Endpoint{Node: toNode, Port: toPort},
	}
}

func graphOf(nodes []workflow.Node, edges []workflow.Edge) workflow.Graph {
	return workflow.Graph{SchemaVersion: workflow.SchemaVersion, Nodes: nodes, Edges: edges}
}

func codesIn(result validate.Result) []string {
	var found []string
	for _, diagnostic := range result.Diagnostics {
		found = append(found, diagnostic.Code)
	}
	return found
}

func hasCode(result validate.Result, code string) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func requireNoCode(t *testing.T, result validate.Result, code string) {
	t.Helper()
	if hasCode(result, code) {
		t.Errorf("did not expect %s; diagnostics = %v", code, codesIn(result))
	}
}

func requireCode(t *testing.T, result validate.Result, code string) {
	t.Helper()
	if !hasCode(result, code) {
		t.Errorf("expected %s; diagnostics = %v", code, codesIn(result))
	}
}

// TestStraightLineDataEdgeIsAllowed is the baseline: an upstream node feeding
// a downstream one on the only path there.
func TestStraightLineDataEdgeIsAllowed(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{
			node("start", "t/start"), node("a", "t/task"), node("b", "t/task"),
		},
		[]workflow.Edge{
			control("c1", "start", "next", "a", "in"),
			control("c2", "a", "next", "b", "in"),
			data("d1", "a", "result", "b", "value"),
		},
	)

	result := validate.Graph(graph, testCatalog(t))
	if !result.Runnable() {
		t.Fatalf("expected a runnable graph, got %v", codesIn(result))
	}
}

// TestParallelBranchToAfterJoinIsAllowed is the case classical dominance gets
// wrong, and the reason the meet operator unions at joins.
//
// A is not on every path from the start to X — the path through B misses it —
// so a dominator-based check would reject this edge. But both branches always
// run and the Join waits for both, so A provably completed before X started.
func TestParallelBranchToAfterJoinIsAllowed(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{
			node("start", "t/start"), node("par", "t/parallel"),
			node("a", "t/task"), node("b", "t/task"),
			node("join", "t/join"), node("x", "t/task"),
		},
		[]workflow.Edge{
			control("c1", "start", "next", "par", "in"),
			control("c2", "par", "branch1", "a", "in"),
			control("c3", "par", "branch2", "b", "in"),
			control("c4", "a", "next", "join", "branch1"),
			control("c5", "b", "next", "join", "branch2"),
			control("c6", "join", "next", "x", "in"),
			data("d1", "a", "result", "x", "value"),
		},
	)

	result := validate.Graph(graph, testCatalog(t))
	requireNoCode(t, result, "data.notGuaranteed")
	requireNoCode(t, result, "data.parallelSiblings")
	if !result.Runnable() {
		t.Fatalf("expected a runnable graph, got %v", codesIn(result))
	}
}

// TestSiblingParallelBranchesAreRejected covers the race the previous test
// must not be confused with.
func TestSiblingParallelBranchesAreRejected(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{
			node("start", "t/start"), node("par", "t/parallel"),
			node("a", "t/task"), node("b", "t/task"), node("join", "t/join"),
		},
		[]workflow.Edge{
			control("c1", "start", "next", "par", "in"),
			control("c2", "par", "branch1", "a", "in"),
			control("c3", "par", "branch2", "b", "in"),
			control("c4", "a", "next", "join", "branch1"),
			control("c5", "b", "next", "join", "branch2"),
			data("d1", "a", "result", "b", "value"),
		},
	)

	result := validate.Graph(graph, testCatalog(t))
	requireCode(t, result, "data.parallelSiblings")
}

// TestConditionalBranchToAfterMergeIsRejected: a node on one arm of a branch
// does not run on every path to the merge point.
func TestConditionalBranchToAfterMergeIsRejected(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{
			node("start", "t/start"), node("branch", "t/branch"),
			node("yes", "t/task"), node("no", "t/task"), node("merge", "t/task"),
		},
		[]workflow.Edge{
			control("c1", "start", "next", "branch", "in"),
			control("c2", "branch", "yes", "yes", "in"),
			control("c3", "branch", "no", "no", "in"),
			control("c4", "yes", "next", "merge", "in"),
			control("c5", "no", "next", "merge", "in"),
			data("d1", "yes", "result", "merge", "value"),
		},
	)

	result := validate.Graph(graph, testCatalog(t))
	requireCode(t, result, "data.notGuaranteed")

	// The diagnostic must carry a concrete path that avoids the source,
	// because "dominance violation" on its own tells a user nothing.
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code != "data.notGuaranteed" {
			continue
		}
		if len(diagnostic.WitnessPath) == 0 {
			t.Error("expected a witness path demonstrating the skipping route")
		}
		for _, nodeID := range diagnostic.WitnessPath {
			if nodeID == "yes" {
				t.Error("witness path must avoid the source node")
			}
		}
	}
}

// TestLoopBodyValueCannotEscapeTheLoop: the loop may run zero times, so no
// single value is guaranteed to exist afterwards.
func TestLoopBodyValueCannotEscapeTheLoop(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{
			node("start", "t/start"), node("list", "t/listSource"),
			node("loop", "t/forEach"), node("body", "t/task"), node("after", "t/task"),
		},
		[]workflow.Edge{
			control("c1", "start", "next", "loop", "in"),
			control("c2", "loop", "body", "body", "in"),
			control("c3", "loop", "done", "after", "in"),
			data("d1", "list", "files", "loop", "collection"),
			data("d2", "body", "result", "after", "value"),
		},
	)

	result := validate.Graph(graph, testCatalog(t))
	requireCode(t, result, "data.escapesLoop")
}

// TestCollectMakesLoopValuesReadableAfterTheLoop: the same wiring becomes
// legal through a Collect, whose output is attributed to the loop's done.
func TestCollectMakesLoopValuesReadableAfterTheLoop(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{
			node("start", "t/start"), node("list", "t/listSource"),
			node("loop", "t/forEach"), node("body", "t/task"),
			node("collect", "t/collect"), node("after", "t/task"),
		},
		[]workflow.Edge{
			control("c1", "start", "next", "loop", "in"),
			control("c2", "loop", "body", "body", "in"),
			control("c3", "body", "next", "collect", "in"),
			control("c4", "loop", "done", "after", "in"),
			data("d1", "list", "files", "loop", "collection"),
			data("d2", "body", "result", "collect", "value"),
			data("d3", "collect", "collected", "after", "value"),
		},
	)

	result := validate.Graph(graph, testCatalog(t))
	requireNoCode(t, result, "data.escapesLoop")
	requireNoCode(t, result, "data.notGuaranteed")
}

// TestPureSourceIsExemptFromRunOrdering: a literal has no control ports and
// is not an execution step, so it may feed anything.
func TestPureSourceIsExemptFromRunOrdering(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{
			node("start", "t/start"), node("branch", "t/branch"),
			node("yes", "t/task"), node("literal", "t/source"),
		},
		[]workflow.Edge{
			control("c1", "start", "next", "branch", "in"),
			control("c2", "branch", "yes", "yes", "in"),
			data("d1", "literal", "path", "yes", "value"),
		},
	)

	result := validate.Graph(graph, testCatalog(t))
	requireNoCode(t, result, "data.notGuaranteed")
}

// TestControlOutPortTakesOnlyOneEdge: implicit fan-out is what would make
// join arity unknowable, so a second wire is refused outright.
func TestControlOutPortTakesOnlyOneEdge(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{
			node("start", "t/start"), node("a", "t/task"), node("b", "t/task"),
		},
		[]workflow.Edge{
			control("c1", "start", "next", "a", "in"),
			control("c2", "start", "next", "b", "in"),
		},
	)

	result := validate.Graph(graph, testCatalog(t))
	requireCode(t, result, "arity.controlOut")
}

// TestDataInSocketTakesOnlyOneEdge: two sources would make the received
// value ambiguous.
func TestDataInSocketTakesOnlyOneEdge(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{
			node("start", "t/start"), node("one", "t/source"), node("two", "t/source"),
			node("task", "t/task"),
		},
		[]workflow.Edge{
			control("c1", "start", "next", "task", "in"),
			data("d1", "one", "path", "task", "value"),
			data("d2", "two", "path", "task", "value"),
		},
	)

	result := validate.Graph(graph, testCatalog(t))
	requireCode(t, result, "arity.dataIn")
}

// TestFileToDirectoryNeedsAnExplicitTransform is the case that motivated
// typing: passing a file where a directory is wanted silently means the
// parent, so it must be visible on the wire rather than implicit.
func TestFileToDirectoryNeedsAnExplicitTransform(t *testing.T) {
	nodes := []workflow.Node{
		node("start", "t/start"), node("literal", "t/source"), node("dir", "t/dirTask"),
	}
	edges := []workflow.Edge{
		control("c1", "start", "next", "dir", "in"),
		data("d1", "literal", "path", "dir", "folder"),
	}

	withoutTransform := validate.Graph(graphOf(nodes, edges), testCatalog(t))
	requireCode(t, withoutTransform, "data.transformRequired")

	edges[1].Transform = "parentDir"
	withTransform := validate.Graph(graphOf(nodes, edges), testCatalog(t))
	requireNoCode(t, withTransform, "data.transformRequired")
	if !withTransform.Runnable() {
		t.Fatalf("expected parentDir to make the graph runnable, got %v", codesIn(withTransform))
	}
}

// TestForEachItemTypeIsInferredFromItsCollection keeps loops properly typed
// rather than degrading everything downstream to "any".
func TestForEachItemTypeIsInferredFromItsCollection(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{
			node("start", "t/start"), node("list", "t/listSource"),
			node("loop", "t/forEach"), node("dir", "t/dirTask"),
		},
		[]workflow.Edge{
			control("c1", "start", "next", "loop", "in"),
			control("c2", "loop", "body", "dir", "in"),
			data("d1", "list", "files", "loop", "collection"),
			// item resolves to path.file, which cannot feed a path.dir
			// socket without parentDir.
			data("d2", "loop", "item", "dir", "folder"),
		},
	)

	result := validate.Graph(graph, testCatalog(t))
	requireCode(t, result, "data.transformRequired")

	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "data.transformRequired" && !strings.Contains(diagnostic.Message, "path.file") {
			t.Errorf("expected the inferred element type in the message, got %q", diagnostic.Message)
		}
	}
}

// TestTerminalInsideParallelBranchIsRejected: ending the run inside a branch
// would leave the join waiting forever.
func TestTerminalInsideParallelBranchIsRejected(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{
			node("start", "t/start"), node("par", "t/parallel"),
			node("a", "t/task"), node("stop", "t/end"),
			node("b", "t/task"), node("join", "t/join"),
		},
		[]workflow.Edge{
			control("c1", "start", "next", "par", "in"),
			control("c2", "par", "branch1", "a", "in"),
			control("c3", "a", "next", "stop", "in"),
			control("c4", "par", "branch2", "b", "in"),
			control("c5", "b", "next", "join", "branch2"),
		},
	)

	result := validate.Graph(graph, testCatalog(t))
	requireCode(t, result, "terminal.inParallelBranch")
}

// TestTerminalAfterJoinIsAllowed guards a false positive: a branch region
// must stop at the join that closes it. Traversing through the join swallows
// the rest of the workflow, so an ordinary End placed after it looks like it
// sits inside a branch.
func TestTerminalAfterJoinIsAllowed(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{
			node("start", "t/start"), node("par", "t/parallel"),
			node("a", "t/task"), node("b", "t/task"),
			node("join", "t/join"), node("stop", "t/end"),
		},
		[]workflow.Edge{
			control("c1", "start", "next", "par", "in"),
			control("c2", "par", "branch1", "a", "in"),
			control("c3", "par", "branch2", "b", "in"),
			control("c4", "a", "next", "join", "branch1"),
			control("c5", "b", "next", "join", "branch2"),
			control("c6", "join", "next", "stop", "in"),
		},
	)

	result := validate.Graph(graph, testCatalog(t))
	requireNoCode(t, result, "terminal.inParallelBranch")
	if !result.Runnable() {
		t.Fatalf("expected a runnable graph, got %v", codesIn(result))
	}
}

// TestMissingStartIsRejected.
func TestMissingStartIsRejected(t *testing.T) {
	graph := graphOf([]workflow.Node{node("a", "t/task")}, nil)

	result := validate.Graph(graph, testCatalog(t))
	requireCode(t, result, "start.missing")
}

// TestUnknownNodeTypeIsReportedNotDropped: the node must survive so the flow
// can be opened and re-saved without losing the user's work.
func TestUnknownNodeTypeIsReportedNotDropped(t *testing.T) {
	graph := graphOf(
		[]workflow.Node{node("start", "t/start"), node("mystery", "t/doesNotExist")},
		nil,
	)

	result := validate.Graph(graph, testCatalog(t))
	requireCode(t, result, "type.missing")
}

// TestLegacySchemaIsRefusedRatherThanGuessed: version 0 documents predate
// control edges entirely, and a guessed migration would produce flows that
// look right and run wrong.
func TestLegacySchemaIsRefusedRatherThanGuessed(t *testing.T) {
	graph := workflow.Graph{SchemaVersion: 0, Nodes: []workflow.Node{node("start", "t/start")}}

	result := validate.Graph(graph, testCatalog(t))
	requireCode(t, result, "schema.legacy")
}
