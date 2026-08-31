package validate

import (
	"fmt"

	"Metarr/internal/shared/workflow"
)

// checkDataEdges applies the three independent rules every data edge must
// satisfy: the source must be guaranteed to have run, its value must be
// visible in the target's loop scope, and the types must line up.
func (a *analysis) checkDataEdges() {
	a.computeLoopScopes()
	a.computeMustHaveRun()

	for _, edge := range workflow.DataEdges(a.graph) {
		sourceType, sourceResolved := a.types[edge.From.Node]
		targetType, targetResolved := a.types[edge.To.Node]
		if !sourceResolved || !targetResolved {
			continue
		}

		a.checkDataEdgeAvailability(edge, sourceType)
		a.checkDataEdgeScope(edge)
		a.checkDataEdgeTypes(edge, targetType)
	}
}

// checkDataEdgeAvailability is the MustHaveRun check.
func (a *analysis) checkDataEdgeAvailability(edge *workflow.Edge, sourceType *workflow.NodeType) {
	// A pure data source is not an execution step — a literal or a selector
	// always has its value — so it is exempt. Without this carve-out the
	// whole relation falls apart on constants.
	if sourceType.Kind == workflow.KindSource {
		return
	}
	if a.mustHaveRun == nil {
		return // no single start node; already reported
	}

	guaranteed, known := a.mustHaveRun[edge.To.Node]
	if !known || guaranteed[a.availabilityProducer(edge.From.Node)] {
		return
	}

	sourceLabel := a.nodeLabel(edge.From.Node)
	targetLabel := a.nodeLabel(edge.To.Node)

	// Sibling parallel branches are the common case and deserve their own
	// wording — "does not always run" would be actively misleading when the
	// real problem is that the two run at the same time.
	if parallelID, siblings := a.parallelSiblings(edge.From.Node, edge.To.Node); siblings {
		a.report(SeverityError, "data.parallelSiblings",
			fmt.Sprintf("%s and %s run at the same time in different branches of %s, so %s cannot read from %s. Connect from a node before the Parallel, or from after the Join.",
				sourceLabel, targetLabel, a.nodeLabel(parallelID), targetLabel, sourceLabel),
			[]string{edge.From.Node, edge.To.Node, parallelID}, []string{edge.Id})
		return
	}

	diagnostic := &Diagnostic{
		Severity: SeverityError,
		Code:     "data.notGuaranteed",
		Message: fmt.Sprintf("%s does not run on every path to %s, so the value may not exist when it is needed.",
			sourceLabel, targetLabel),
		NodeIds:     []string{edge.From.Node, edge.To.Node},
		EdgeIds:     []string{edge.Id},
		WitnessPath: a.witnessPath(edge.To.Node, edge.From.Node),
	}
	a.diagnostics = append(a.diagnostics, diagnostic)
}

// availabilityProducer is the node that MustHaveRun should be asked about on
// behalf of a value's source.
//
// For everything except a collect this is the source itself. A collect's
// collected value is attributed to the enclosing loop's done transition — for
// availability purposes the producer *is* the loop header — which is what
// makes a value legitimately readable after a loop that might have run zero
// times, without needing an exception anywhere in the analysis itself.
func (a *analysis) availabilityProducer(nodeID string) string {
	a.computeLoopScopes()
	if nodeType, resolved := a.types[nodeID]; resolved && nodeType.Kind == workflow.KindCollect {
		if enclosingLoop := a.loopScope[nodeID]; enclosingLoop != "" {
			return enclosingLoop
		}
	}
	return nodeID
}

// parallelSiblings reports whether two nodes sit in different branches of the
// same parallel.
func (a *analysis) parallelSiblings(left, right string) (string, bool) {
	for _, node := range a.graph.Nodes {
		nodeType, resolved := a.types[node.Id]
		if !resolved || nodeType.Kind != workflow.KindParallel {
			continue
		}

		leftBranch, rightBranch := "", ""
		for _, port := range nodeType.Control.Out {
			// Bounded at the join: two nodes after the join are not siblings,
			// they are sequential.
			reached := a.branchRegion(node.Id, port)
			if reached[left] {
				leftBranch = port
			}
			if reached[right] {
				rightBranch = port
			}
		}
		if leftBranch != "" && rightBranch != "" && leftBranch != rightBranch {
			return node.Id, true
		}
	}
	return "", false
}

// checkDataEdgeScope is the frame-visibility check. It is not implied by
// MustHaveRun: that establishes some value exists, this establishes that
// *this iteration's* value does.
func (a *analysis) checkDataEdgeScope(edge *workflow.Edge) {
	sourceScope := a.effectiveScope(edge.From.Node)
	targetScope := a.loopScope[edge.To.Node]
	if a.scopeVisible(sourceScope, targetScope) {
		return
	}

	sourceLabel := a.nodeLabel(edge.From.Node)
	targetLabel := a.nodeLabel(edge.To.Node)
	loopLabel := a.nodeLabel(sourceScope)

	a.report(SeverityError, "data.escapesLoop",
		fmt.Sprintf("%s runs once per item inside %s, but %s is outside that loop, so there is no single value to read — the loop may even run zero times. Insert a Collect node inside the loop and connect that instead.",
			sourceLabel, loopLabel, targetLabel),
		[]string{edge.From.Node, edge.To.Node, sourceScope}, []string{edge.Id})
}

// checkDataEdgeTypes verifies the value can reach the socket, applying the
// edge's recorded transform if it has one.
func (a *analysis) checkDataEdgeTypes(edge *workflow.Edge, targetType *workflow.NodeType) {
	targetSocket, found := workflow.DataInSocket(targetType, edge.To.Port)
	if !found {
		return // already reported by checkPortsExist
	}
	targetSocketType := workflow.Type(targetSocket.Type)

	producedType := a.resolveOutputType(edge.From.Node, edge.From.Port, nil)

	if edge.Transform != "" {
		transform, known := workflow.ResolveTransform(edge.Transform, producedType, targetSocketType)
		if !known {
			a.report(SeverityError, "data.unknownTransform",
				fmt.Sprintf("This connection uses a conversion called %q, which this build does not know about.", edge.Transform),
				nil, []string{edge.Id})
			return
		}
		if !workflow.IsSubtypeOf(producedType, workflow.Type(transform.From)) {
			a.report(SeverityError, "data.transformInputMismatch",
				fmt.Sprintf("The %s conversion expects %s but receives %s.", transform.Name, transform.From, producedType),
				nil, []string{edge.Id})
			return
		}
		if !workflow.IsSubtypeOf(workflow.Type(transform.To), targetSocketType) {
			a.report(SeverityError, "data.transformOutputMismatch",
				fmt.Sprintf("The %s conversion produces %s, which %q does not accept.", transform.Name, transform.To, targetSocket.Name),
				nil, []string{edge.Id})
		}
		return
	}

	connection := workflow.CanConnect(producedType, targetSocketType)
	if connection.Direct {
		return
	}
	if len(connection.Candidates) > 0 {
		names := make([]string, 0, len(connection.Candidates))
		for _, candidate := range connection.Candidates {
			names = append(names, candidate.Name)
		}
		a.report(SeverityError, "data.transformRequired",
			fmt.Sprintf("%s cannot feed %q (%s) directly. Choose a conversion: %v.",
				producedType, targetSocket.Name, targetSocket.Type, names),
			nil, []string{edge.Id})
		return
	}

	a.report(SeverityError, "data.incompatible",
		workflow.ExplainIncompatible(producedType, targetSocketType),
		nil, []string{edge.Id})
}

// resolveOutputType returns the concrete type a data-out port produces.
//
// Two ports are declared with a placeholder in the catalog and resolved from
// what is wired in, so that loops type-check properly rather than degrading
// everything downstream to "any": a For Each's item is the element type of
// its collection, and a Collect's collected is a list of whatever it is
// given. visiting guards against recursion on a malformed graph.
func (a *analysis) resolveOutputType(nodeID, port string, visiting map[string]bool) workflow.Type {
	nodeType, resolved := a.types[nodeID]
	if !resolved {
		return workflow.TypeAny
	}

	key := nodeID + "\x00" + port
	if visiting[key] {
		return workflow.TypeAny
	}
	if visiting == nil {
		visiting = make(map[string]bool)
	}
	visiting[key] = true

	switch nodeType.Kind {
	case workflow.KindForEach:
		if port == "item" {
			collection, wired := a.wiredInputType(nodeID, "collection", visiting)
			if wired {
				if element, isList := collection.ElementType(); isList {
					return element
				}
			}
			return workflow.TypeAny
		}
	case workflow.KindCollect:
		if port == "collected" {
			value, wired := a.wiredInputType(nodeID, "value", visiting)
			if wired {
				return workflow.ListOf(value)
			}
			return workflow.ListOf(workflow.TypeAny)
		}
	}

	if socket, found := workflow.DataOutSocket(nodeType, port); found {
		return workflow.Type(socket.Type)
	}
	return workflow.TypeAny
}

// wiredInputType is the type actually arriving at a data-in socket, after any
// transform on the incoming edge.
func (a *analysis) wiredInputType(nodeID, port string, visiting map[string]bool) (workflow.Type, bool) {
	for _, edge := range workflow.DataEdges(a.graph) {
		if edge.To.Node != nodeID || edge.To.Port != port {
			continue
		}
		producedType := a.resolveOutputType(edge.From.Node, edge.From.Port, visiting)
		if edge.Transform != "" {
			targetType := workflow.TypeAny
			if nodeType, resolved := a.types[nodeID]; resolved {
				if socket, found := workflow.DataInSocket(nodeType, port); found {
					targetType = workflow.Type(socket.Type)
				}
			}
			if transform, known := workflow.ResolveTransform(edge.Transform, producedType, targetType); known {
				return workflow.Type(transform.To), true
			}
		}
		return producedType, true
	}
	return "", false
}
