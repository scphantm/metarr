package validate

import (
	"fmt"
	"sort"

	"Metarr/internal/shared/workflow"
)

// controlSuccessors maps node id to the ids its control edges lead to,
// restricted to executable nodes.
func (a *analysis) controlSuccessors() map[string][]string {
	successors := make(map[string][]string)
	for _, edge := range workflow.ControlEdges(a.graph) {
		if a.executable(edge.From.Node) && a.executable(edge.To.Node) {
			successors[edge.From.Node] = append(successors[edge.From.Node], edge.To.Node)
		}
	}
	return successors
}

// reachableFromPort returns every node reachable by control flow starting
// from the edges leaving one specific out-port.
func (a *analysis) reachableFromPort(nodeID, port string) map[string]bool {
	return a.reachableFromPortBounded(nodeID, port, false)
}

// branchRegion returns the nodes inside one branch of a parallel: everything
// from the branch port up to, but not through, the join that closes it.
//
// The bound matters. Without it the traversal runs straight through the join
// and swallows the whole rest of the workflow, so a perfectly ordinary End
// placed after the join looks like it sits inside a branch.
func (a *analysis) branchRegion(parallelID, port string) map[string]bool {
	return a.reachableFromPortBounded(parallelID, port, true)
}

func (a *analysis) reachableFromPortBounded(nodeID, port string, stopAtJoins bool) map[string]bool {
	successors := a.controlSuccessors()

	var queue []string
	for _, edge := range workflow.ControlEdges(a.graph) {
		if edge.From.Node == nodeID && edge.From.Port == port {
			queue = append(queue, edge.To.Node)
		}
	}

	reached := make(map[string]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if reached[current] {
			continue
		}
		reached[current] = true

		if stopAtJoins {
			if currentType, resolved := a.types[current]; resolved && currentType.Kind == workflow.KindJoin {
				// The join closes the branch; whatever follows it is
				// sequential again and belongs to nobody's branch.
				continue
			}
		}
		queue = append(queue, successors[current]...)
	}
	return reached
}

// computeLoopScopes assigns every node the innermost loop whose body
// contains it, or "" when it is top-level.
//
// Because both body and done leave the forEach node itself, the body region
// is simply what is reachable from body — and a node after the loop is
// reachable only from done, so it correctly falls outside. That is what makes
// the zero-iteration case fall out of the analysis instead of needing a rule.
func (a *analysis) computeLoopScopes() {
	if a.loopScope != nil {
		return
	}
	a.loopScope = make(map[string]string)

	regions := make(map[string]map[string]bool)
	for _, node := range a.graph.Nodes {
		if nodeType, resolved := a.types[node.Id]; resolved && nodeType.Kind == workflow.KindForEach {
			regions[node.Id] = a.reachableFromPort(node.Id, "body")
		}
	}

	for _, node := range a.graph.Nodes {
		innermost := ""
		smallest := -1
		for loopID, region := range regions {
			if !region[node.Id] {
				continue
			}
			// Innermost wins: a node inside a nested loop is inside both
			// regions, and the smaller region is the inner one.
			if smallest == -1 || len(region) < smallest {
				innermost = loopID
				smallest = len(region)
			}
		}
		a.loopScope[node.Id] = innermost
	}
}

// effectiveScope is the loop scope a node's *outputs* are visible in.
//
// A collect node is the one case where this differs from where the node
// itself sits: its collected value is attributed to the enclosing loop's done
// transition, so it is readable after the loop rather than only inside it.
// Making that an attribution rule rather than a special case in the checks is
// what keeps the rest of the analysis free of loop-specific exceptions.
func (a *analysis) effectiveScope(nodeID string) string {
	a.computeLoopScopes()
	nodeType, resolved := a.types[nodeID]
	if resolved && nodeType.Kind == workflow.KindCollect {
		enclosingLoop := a.loopScope[nodeID]
		if enclosingLoop == "" {
			return ""
		}
		return a.loopScope[enclosingLoop]
	}
	return a.loopScope[nodeID]
}

// scopeVisible reports whether a value produced in sourceScope can be read in
// targetScope — true when sourceScope is an ancestor of, or equal to,
// targetScope in the loop-nesting tree.
func (a *analysis) scopeVisible(sourceScope, targetScope string) bool {
	if sourceScope == "" {
		return true // top-level values are visible everywhere
	}
	for scope := targetScope; scope != ""; scope = a.loopScope[scope] {
		if scope == sourceScope {
			return true
		}
	}
	return false
}

// computeMustHaveRun solves the "is the source guaranteed to have executed
// before the target" relation.
//
// Classical dominance is the wrong relation here: it models fan-out as a
// choice, but a Parallel's fan-out is a concurrency. In
// Parallel -> {A, B} -> Join -> X, A is not on every path from the start to X
// (the path through B misses it), so dominance would reject a data edge
// A -> X even though both branches always run and the Join waits for both.
//
// The correction is the meet operator: intersect at ordinary merge points, as
// dominance does, but *union* at joins, because every branch of a join ran.
func (a *analysis) computeMustHaveRun() {
	if a.mustHaveRun != nil {
		return
	}
	a.mustHaveRun = make(map[string]map[string]bool)

	start := a.startNode()
	if start == "" {
		return // already reported; nothing meaningful to compute
	}

	var universe []string
	for _, node := range a.graph.Nodes {
		if a.executable(node.Id) {
			universe = append(universe, node.Id)
		}
	}
	sort.Strings(universe)

	inbound := make(map[string][]*workflow.Edge)
	for _, edge := range workflow.ControlEdges(a.graph) {
		if a.executable(edge.From.Node) && a.executable(edge.To.Node) {
			inbound[edge.To.Node] = append(inbound[edge.To.Node], edge)
		}
	}

	// The start knows nothing ran before it; everything else begins
	// maximally optimistic and shrinks to a fixed point, which is what makes
	// the intersection meet converge.
	for _, nodeID := range universe {
		if nodeID == start {
			a.mustHaveRun[nodeID] = map[string]bool{}
			continue
		}
		full := make(map[string]bool, len(universe))
		for _, other := range universe {
			full[other] = true
		}
		a.mustHaveRun[nodeID] = full
	}

	for changed := true; changed; {
		changed = false
		for _, nodeID := range universe {
			if nodeID == start {
				continue
			}
			edges := inbound[nodeID]
			if len(edges) == 0 {
				continue // unreachable; reported separately
			}

			var updated map[string]bool
			if nodeType, resolved := a.types[nodeID]; resolved && nodeType.Kind == workflow.KindJoin {
				updated = a.joinMeet(edges)
			} else {
				updated = a.ordinaryMeet(edges)
			}
			if !sameSet(updated, a.mustHaveRun[nodeID]) {
				a.mustHaveRun[nodeID] = updated
				changed = true
			}
		}
	}
}

// through is MustHaveRun(source) plus the source itself — what is guaranteed
// to have run when control arrives along this edge.
func (a *analysis) through(edge *workflow.Edge) map[string]bool {
	result := make(map[string]bool, len(a.mustHaveRun[edge.From.Node])+1)
	for nodeID := range a.mustHaveRun[edge.From.Node] {
		result[nodeID] = true
	}
	result[edge.From.Node] = true
	return result
}

// ordinaryMeet intersects: only one predecessor actually ran.
func (a *analysis) ordinaryMeet(edges []*workflow.Edge) map[string]bool {
	result := a.through(edges[0])
	for _, edge := range edges[1:] {
		result = intersect(result, a.through(edge))
	}
	return result
}

// joinMeet intersects within each branch and unions across them, because a
// join only proceeds once every branch has arrived.
func (a *analysis) joinMeet(edges []*workflow.Edge) map[string]bool {
	byBranch := make(map[string][]*workflow.Edge)
	for _, edge := range edges {
		byBranch[edge.To.Port] = append(byBranch[edge.To.Port], edge)
	}

	result := make(map[string]bool)
	for _, branchEdges := range byBranch {
		branchResult := a.through(branchEdges[0])
		for _, edge := range branchEdges[1:] {
			branchResult = intersect(branchResult, a.through(edge))
		}
		for nodeID := range branchResult {
			result[nodeID] = true
		}
	}
	return result
}

func intersect(left, right map[string]bool) map[string]bool {
	result := make(map[string]bool)
	for nodeID := range left {
		if right[nodeID] {
			result[nodeID] = true
		}
	}
	return result
}

func sameSet(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for nodeID := range left {
		if !right[nodeID] {
			return false
		}
	}
	return true
}

// witnessPath finds a control path from the start to target that avoids the
// given node, demonstrating concretely why a data edge is unsafe.
func (a *analysis) witnessPath(target, avoid string) []string {
	start := a.startNode()
	if start == "" || start == avoid {
		return nil
	}

	successors := a.controlSuccessors()
	cameFrom := map[string]string{start: ""}
	queue := []string{start}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == target {
			var path []string
			for node := target; node != ""; node = cameFrom[node] {
				path = append([]string{node}, path...)
			}
			return path
		}
		for _, next := range successors[current] {
			if next == avoid {
				continue
			}
			if _, seen := cameFrom[next]; seen {
				continue
			}
			cameFrom[next] = current
			queue = append(queue, next)
		}
	}
	return nil
}

// nodeLabel is what a diagnostic calls a node: the author's own label if they
// set one, otherwise the catalog name.
func (a *analysis) nodeLabel(nodeID string) string {
	if node, found := a.nodes[nodeID]; found && node.Label != "" {
		return node.Label
	}
	if nodeType, resolved := a.types[nodeID]; resolved {
		return nodeType.Name
	}
	return nodeID
}

// checkTerminalPlacement enforces that a run cannot be ended from inside a
// parallel branch or a loop body. This is not stylistic: it is what
// guarantees every branch reaches its join exactly once, which is what makes
// join arity static and deadlock impossible.
func (a *analysis) checkTerminalPlacement() {
	a.computeLoopScopes()

	parallelBranchNodes := make(map[string]string)
	for _, node := range a.graph.Nodes {
		nodeType, resolved := a.types[node.Id]
		if !resolved || nodeType.Kind != workflow.KindParallel {
			continue
		}
		for _, port := range nodeType.Control.Out {
			for reachedNode := range a.branchRegion(node.Id, port) {
				if reachedType, ok := a.types[reachedNode]; ok && reachedType.Kind == workflow.KindJoin {
					continue
				}
				parallelBranchNodes[reachedNode] = node.Id
			}
		}
	}

	for _, node := range a.graph.Nodes {
		nodeType, resolved := a.types[node.Id]
		if !resolved {
			continue
		}
		isTerminal := nodeType.Kind == workflow.KindEnd || nodeType.Kind == workflow.KindFail
		if !isTerminal {
			continue
		}

		if loopID := a.loopScope[node.Id]; loopID != "" {
			a.report(SeverityError, "terminal.inLoop",
				fmt.Sprintf("%s ends the run, so it cannot sit inside the body of %s. Use a Break node to leave the loop instead.",
					a.nodeLabel(node.Id), a.nodeLabel(loopID)),
				[]string{node.Id, loopID}, nil)
			continue
		}
		if parallelID, inBranch := parallelBranchNodes[node.Id]; inBranch {
			a.report(SeverityError, "terminal.inParallelBranch",
				fmt.Sprintf("%s ends the run, so it cannot sit inside a branch of %s — the other branches would never reach the Join. Route to the Join and end after it.",
					a.nodeLabel(node.Id), a.nodeLabel(parallelID)),
				[]string{node.Id, parallelID}, nil)
		}
	}
}

// checkParallelJoins verifies that every parallel's branches converge on one
// shared join, and that no more branches are wired than the node declares.
func (a *analysis) checkParallelJoins() {
	for _, node := range a.graph.Nodes {
		nodeType, resolved := a.types[node.Id]
		if !resolved || nodeType.Kind != workflow.KindParallel {
			continue
		}

		usedPorts := map[string]bool{}
		for _, edge := range workflow.ControlEdges(a.graph) {
			if edge.From.Node == node.Id {
				usedPorts[edge.From.Port] = true
			}
		}
		if len(usedPorts) == 0 {
			continue
		}
		if len(usedPorts) < 2 {
			a.report(SeverityWarning, "parallel.singleBranch",
				fmt.Sprintf("%s has only one branch wired, so it runs nothing concurrently.", a.nodeLabel(node.Id)),
				[]string{node.Id}, nil)
		}
		if declared := declaredBranches(a.nodes[node.Id], nodeType); declared > 0 && len(usedPorts) > declared {
			a.report(SeverityError, "parallel.tooManyBranches",
				fmt.Sprintf("%s is set to %d branches but %d are wired.", a.nodeLabel(node.Id), declared, len(usedPorts)),
				[]string{node.Id}, nil)
		}

		var shared map[string]bool
		for port := range usedPorts {
			joins := map[string]bool{}
			for reached := range a.reachableFromPort(node.Id, port) {
				if reachedType, ok := a.types[reached]; ok && reachedType.Kind == workflow.KindJoin {
					joins[reached] = true
				}
			}
			if shared == nil {
				shared = joins
				continue
			}
			shared = intersect(shared, joins)
		}
		if len(shared) == 0 {
			a.report(SeverityError, "parallel.noSharedJoin",
				fmt.Sprintf("The branches of %s do not all reach the same Join, so the run could not tell when they had finished.", a.nodeLabel(node.Id)),
				[]string{node.Id}, nil)
		}
	}
}

// declaredBranches reads the parallel's branches setting, falling back to the
// catalog default.
func declaredBranches(node *workflow.Node, nodeType *workflow.NodeType) int {
	if node != nil && node.Settings != nil {
		if raw, set := node.Settings.Fields["branches"]; set {
			if count, ok := asInt(raw.AsInterface()); ok {
				return count
			}
		}
	}
	for _, setting := range nodeType.Settings {
		if setting.Name == "branches" {
			if count, ok := asInt(setting.Default.AsInterface()); ok {
				return count
			}
		}
	}
	return 0
}

func asInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	}
	return 0, false
}
