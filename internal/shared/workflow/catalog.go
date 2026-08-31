package workflow

import (
	"errors"
	"fmt"
	"strings"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// The catalog model. Every type here is an alias to the generated metarr.v1
// message that defines it: proto is the single definition for a model that
// crosses a language boundary, and this one is read by the editor palette,
// server-side validation and the engine. See docs/adr/0005. Behaviour that
// used to hang off these types as methods is package-level functions below.
type (
	NodeType     = metarrv1.WorkflowNodeType
	ControlPorts = metarrv1.WorkflowControlPorts
	Socket       = metarrv1.WorkflowSocket
	Setting      = metarrv1.WorkflowSetting
	RetrySpec    = metarrv1.WorkflowRetrySpec
	ExecSpec     = metarrv1.WorkflowExecSpec
)

// NodeKind tells the validator and engine how a node participates in control
// flow. It is declared rather than inferred from the type name, because the
// MustHaveRun analysis switches on whether a node is a join and guessing that
// from a string would be fragile. The engine owns the vocabulary and it is
// closed, so it is a generated enum.
type NodeKind = metarrv1.WorkflowNodeKind

const (
	// KindTask is an ordinary step. It is the zero value, so a catalog entry
	// that says nothing is a plain task.
	KindTask NodeKind = metarrv1.WorkflowNodeKind_WORKFLOW_NODE_KIND_UNSPECIFIED
	// KindStart is the single entry point of a workflow.
	KindStart NodeKind = metarrv1.WorkflowNodeKind_WORKFLOW_NODE_KIND_START
	// KindEnd terminates a run. Forbidden inside parallel branches and loop
	// bodies.
	KindEnd NodeKind = metarrv1.WorkflowNodeKind_WORKFLOW_NODE_KIND_END
	// KindFail terminates a run as failed.
	KindFail NodeKind = metarrv1.WorkflowNodeKind_WORKFLOW_NODE_KIND_FAIL
	// KindSource is a pure data source — a literal, a selector, a constant.
	// It has no control ports at all and is exempt from MustHaveRun, because
	// it is not an execution step and its value is always available.
	KindSource NodeKind = metarrv1.WorkflowNodeKind_WORKFLOW_NODE_KIND_SOURCE
	// KindBranch chooses exactly one of several control outs.
	KindBranch NodeKind = metarrv1.WorkflowNodeKind_WORKFLOW_NODE_KIND_BRANCH
	// KindForEach iterates, firing body per item then done once.
	KindForEach NodeKind = metarrv1.WorkflowNodeKind_WORKFLOW_NODE_KIND_FOR_EACH
	// KindCollect accumulates a value inside a loop body. Its output is
	// attributed to the enclosing loop's done transition.
	KindCollect NodeKind = metarrv1.WorkflowNodeKind_WORKFLOW_NODE_KIND_COLLECT
	// KindParallel fans out into concurrent branches.
	KindParallel NodeKind = metarrv1.WorkflowNodeKind_WORKFLOW_NODE_KIND_PARALLEL
	// KindJoin is the barrier paired with a parallel.
	KindJoin NodeKind = metarrv1.WorkflowNodeKind_WORKFLOW_NODE_KIND_JOIN
	// KindBreak terminates the enclosing loop.
	KindBreak NodeKind = metarrv1.WorkflowNodeKind_WORKFLOW_NODE_KIND_BREAK
	// KindNote is an annotation. It has no ports, is stripped before
	// compilation, and is excluded from every validation pass.
	KindNote NodeKind = metarrv1.WorkflowNodeKind_WORKFLOW_NODE_KIND_NOTE
)

// PortKind is the closed two-value vocabulary separating a node's control
// wiring from its data wiring. It is a generated enum: the editor's handle
// ids encode it, and control and data edges are validated by entirely
// different rules.
type PortKind = metarrv1.WorkflowPortKind

const (
	PortControl PortKind = metarrv1.WorkflowPortKind_WORKFLOW_PORT_KIND_CONTROL
	PortData    PortKind = metarrv1.WorkflowPortKind_WORKFLOW_PORT_KIND_DATA
)

// Agent selector strategies for ExecSpec.AgentSelector.
const (
	// AgentSelectorPath derives the agent from which library the node's
	// primary input path belongs to. This is the default and is almost
	// always right, because a filesystem only exists on the machine that has
	// it mounted.
	AgentSelectorPath = "path"
	// AgentSelectorAny permits any online agent. Valid only for nodes that
	// touch no filesystem, which in this domain is nearly none.
	AgentSelectorAny = "any"
	// AgentSelectorSettingPrefix names a setting holding the agent slug,
	// e.g. "setting:agentToRun", for the transcode-on-the-GPU-box case.
	AgentSelectorSettingPrefix = "setting:"
)

// Where a node runs.
const (
	RunsOnServer = "server"
	RunsOnAgent  = "agent"
)

// RunsOnAgentNode reports whether this node must be dispatched to an agent.
func RunsOnAgentNode(n *NodeType) bool {
	return n.Exec != nil && n.Exec.RunsOn == RunsOnAgent
}

// DataInSocket finds a declared data-in socket by name.
func DataInSocket(n *NodeType, name string) (*Socket, bool) {
	return findSocket(n.DataIn, name)
}

// DataOutSocket finds a declared data-out socket by name.
func DataOutSocket(n *NodeType, name string) (*Socket, bool) {
	return findSocket(n.DataOut, name)
}

func findSocket(sockets []*Socket, name string) (*Socket, bool) {
	for _, socket := range sockets {
		if socket.Name == name {
			return socket, true
		}
	}
	return nil, false
}

// HasControlIn reports whether name is a declared control in-port.
func HasControlIn(n *NodeType, name string) bool {
	return n.Control != nil && containsString(n.Control.In, name)
}

// HasControlOut reports whether name is a declared control out-port,
// including the implicit error port.
func HasControlOut(n *NodeType, name string) bool {
	if n.Control == nil {
		return false
	}
	if n.Control.Error && name == ErrorPort {
		return true
	}
	return containsString(n.Control.Out, name)
}

// ErrorPort is the reserved name of every node's error control-out.
const ErrorPort = "error"

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// ValidateNodeType checks a single catalog entry for the invariants the rest
// of the system assumes. A catalog that fails this is a load error rather
// than a warning: a malformed entry would otherwise surface as a confusing
// failure deep inside a run.
func ValidateNodeType(n *NodeType) error {
	var problems []string

	if n.Id == "" {
		problems = append(problems, "id is required")
	}
	if n.Type == "" {
		problems = append(problems, "type is required")
	} else if !strings.Contains(n.Type, "/") {
		problems = append(problems, "type must be namespaced, e.g. core/trickplay")
	}
	if n.Name == "" {
		problems = append(problems, "name is required")
	}

	// Effects is mandatory rather than defaulted: defaulting it would mean
	// guessing whether an undeclared node writes to disk, and guessing wrong
	// in the permissive direction is how a dry run mutates a library.
	if n.Exec == nil {
		problems = append(problems, "exec is required")
	} else {
		if !EffectsValid(n.Exec.Effects) {
			problems = append(problems, "exec.effects must be one of read, write, destructive")
		}
		switch n.Exec.RunsOn {
		case "", RunsOnServer, RunsOnAgent:
		default:
			problems = append(problems, "exec.runsOn must be server or agent")
		}
		if n.Exec.RunsOn == RunsOnAgent {
			selector := n.Exec.AgentSelector
			valid := selector == "" ||
				selector == AgentSelectorPath ||
				selector == AgentSelectorAny ||
				strings.HasPrefix(selector, AgentSelectorSettingPrefix)
			if !valid {
				problems = append(problems, "exec.agentSelector must be path, any, or setting:<name>")
			}
			if settingName, ok := strings.CutPrefix(selector, AgentSelectorSettingPrefix); ok {
				if _, found := findSetting(n.Settings, settingName); !found {
					problems = append(problems, "exec.agentSelector names setting "+settingName+", which this type does not declare")
				}
			}
		}
	}

	problems = append(problems, portProblems(n)...)

	if len(problems) > 0 {
		return fmt.Errorf("node type %s (%s): %s", n.Type, n.Id, strings.Join(problems, "; "))
	}
	return nil
}

// portProblems checks that no two ports on the same node collide. Names must
// be unique within each direction, and the reserved error port must not be
// declared by hand.
func portProblems(n *NodeType) []string {
	var problems []string

	control := n.Control
	if control == nil {
		control = &ControlPorts{}
	}

	if containsString(control.Out, ErrorPort) {
		problems = append(problems, "control.out must not list "+ErrorPort+" — set control.error instead")
	}
	problems = append(problems, duplicateProblems("control.in", control.In)...)
	problems = append(problems, duplicateProblems("control.out", control.Out)...)

	dataInNames := make([]string, 0, len(n.DataIn))
	for _, socket := range n.DataIn {
		dataInNames = append(dataInNames, socket.Name)
		if socket.Type == "" {
			problems = append(problems, "dataIn "+socket.Name+" has no type")
		}
	}
	problems = append(problems, duplicateProblems("dataIn", dataInNames)...)

	dataOutNames := make([]string, 0, len(n.DataOut))
	for _, socket := range n.DataOut {
		dataOutNames = append(dataOutNames, socket.Name)
		if socket.Type == "" {
			problems = append(problems, "dataOut "+socket.Name+" has no type")
		}
	}
	problems = append(problems, duplicateProblems("dataOut", dataOutNames)...)

	settingNames := make([]string, 0, len(n.Settings))
	for _, setting := range n.Settings {
		settingNames = append(settingNames, setting.Name)
	}
	problems = append(problems, duplicateProblems("settings", settingNames)...)

	return problems
}

func duplicateProblems(where string, names []string) []string {
	var problems []string
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if name == "" {
			problems = append(problems, where+" has an entry with no name")
			continue
		}
		if seen[name] {
			problems = append(problems, where+" declares "+name+" more than once")
		}
		seen[name] = true
	}
	return problems
}

func findSetting(settings []*Setting, name string) (*Setting, bool) {
	for _, setting := range settings {
		if setting.Name == name {
			return setting, true
		}
	}
	return nil, false
}

// Catalog is the loaded set of node types, keyed by id.
type Catalog struct {
	entries map[string]*NodeType
	ordered []*NodeType
}

// ErrCatalogEmpty is returned when a catalog contains no entries, which is
// almost certainly a misconfigured path rather than an intentional state.
var ErrCatalogEmpty = errors.New("workflow: catalog contains no node types")

// NewCatalog validates every entry and indexes them. A single bad entry
// fails the whole catalog, so a typo is caught at startup rather than when
// somebody happens to drag that node onto a canvas.
//
// Several entries may share a `type` — that's how a plugin offers variations
// (e.g. two core/start entries with different dataOut shapes) without a new
// registered type per variation. `id` is what has to be unique.
func NewCatalog(entries []*NodeType) (*Catalog, error) {
	if len(entries) == 0 {
		return nil, ErrCatalogEmpty
	}

	indexed := make(map[string]*NodeType, len(entries))
	ordered := make([]*NodeType, 0, len(entries))
	for _, entry := range entries {
		if err := ValidateNodeType(entry); err != nil {
			return nil, err
		}
		if _, duplicate := indexed[entry.Id]; duplicate {
			return nil, fmt.Errorf("workflow: catalog declares id %s more than once", entry.Id)
		}
		indexed[entry.Id] = entry
		ordered = append(ordered, entry)
	}
	return &Catalog{entries: indexed, ordered: ordered}, nil
}

// Lookup finds a node type by the catalog id a stored node instance carries.
func (c *Catalog) Lookup(id string) (*NodeType, bool) {
	entry, found := c.entries[id]
	return entry, found
}

// LookupByType returns the first entry (in catalog-file order) whose Type
// matches. This is a backward-compatibility fallback only, for graph nodes
// saved before catalog entries carried an id — when several entries share a
// type, which one it returns is arbitrary but deterministic, the same
// ambiguity such a save already had.
func (c *Catalog) LookupByType(nodeType string) (*NodeType, bool) {
	for _, entry := range c.ordered {
		if entry.Type == nodeType {
			return entry, true
		}
	}
	return nil, false
}

// All returns every entry in catalog order, for serving to the editor.
func (c *Catalog) All() []*NodeType {
	all := make([]*NodeType, len(c.ordered))
	copy(all, c.ordered)
	return all
}

// Len reports how many node types are loaded.
func (c *Catalog) Len() int { return len(c.entries) }
