package workflow

import (
	"errors"
	"fmt"
	"strings"
)

// NodeKind tells the validator and engine how a node participates in control
// flow. It is declared rather than inferred from the type name, because the
// MustHaveRun analysis switches on whether a node is a join and guessing that
// from a string would be fragile.
type NodeKind string

const (
	// KindTask is an ordinary step. It is the zero value, so a catalog entry
	// that says nothing is a plain task.
	KindTask NodeKind = ""
	// KindStart is the single entry point of a workflow.
	KindStart NodeKind = "start"
	// KindEnd terminates a run. Forbidden inside parallel branches and loop
	// bodies.
	KindEnd NodeKind = "end"
	// KindFail terminates a run as failed.
	KindFail NodeKind = "fail"
	// KindSource is a pure data source — a literal, a selector, a constant.
	// It has no control ports at all and is exempt from MustHaveRun, because
	// it is not an execution step and its value is always available.
	KindSource NodeKind = "source"
	// KindBranch chooses exactly one of several control outs.
	KindBranch NodeKind = "branch"
	// KindForEach iterates, firing body per item then done once.
	KindForEach NodeKind = "forEach"
	// KindCollect accumulates a value inside a loop body. Its output is
	// attributed to the enclosing loop's done transition.
	KindCollect NodeKind = "collect"
	// KindParallel fans out into concurrent branches.
	KindParallel NodeKind = "parallel"
	// KindJoin is the barrier paired with a parallel.
	KindJoin NodeKind = "join"
	// KindBreak terminates the enclosing loop.
	KindBreak NodeKind = "break"
	// KindNote is an annotation. It has no ports, is stripped before
	// compilation, and is excluded from every validation pass.
	KindNote NodeKind = "note"
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

// ControlPorts declares a node type's execution wiring. These are not values.
//
// An empty In is what makes a node a starting point, and an empty Out what
// makes it an ending point — the catalog says so directly rather than the UI
// inferring it from a category name.
type ControlPorts struct {
	In  []string `json:"in"`
	Out []string `json:"out"`
	// Error adds the red error out-port. It is an ordinary control branch;
	// the only thing special about it is how it is drawn and that leaving it
	// unwired aborts the run.
	Error bool `json:"error,omitempty"`
}

// Socket is a typed data port.
type Socket struct {
	// Name is a permanent identifier that stored edges reference. Renaming
	// one silently breaks every saved workflow — change Label instead.
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`
	Type        Type   `json:"type"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

// Setting is a literal the user types into the node's editor. It is never
// wired, unless promoted on a specific instance.
type Setting struct {
	Name        string         `json:"name"`
	Label       string         `json:"label,omitempty"`
	Type        Type           `json:"type"`
	Default     any            `json:"default,omitempty"`
	UI          map[string]any `json:"ui,omitempty"`
	Description string         `json:"description,omitempty"`
}

// RetrySpec governs retries of infrastructure failures — an agent going
// offline, Redis being unavailable, a dispatch timing out. Node errors are
// not retried; they go to the error port.
type RetrySpec struct {
	Attempts int    `json:"attempts,omitempty"`
	Backoff  string `json:"backoff,omitempty"`
}

// ExecSpec says where and how a node runs.
type ExecSpec struct {
	// RunsOn defaults to the server; a node declares "agent" when its work
	// can only happen where the files are.
	RunsOn        string `json:"runsOn,omitempty"`
	AgentSelector string `json:"agentSelector,omitempty"`
	Timeout       string `json:"timeout,omitempty"`
	Cancellable   bool   `json:"cancellable,omitempty"`
	// Effects is mandatory. It is what dry-run keys off.
	Effects Effects   `json:"effects"`
	Retry   RetrySpec `json:"retry,omitempty"`
}

// NodeType is one catalog entry: the definition of a kind of node, as
// distinct from an instance of one placed on a canvas.
type NodeType struct {
	Type        string   `json:"type"`
	TypeVersion string   `json:"typeVersion"`
	Name        string   `json:"name"`
	Category    string   `json:"category,omitempty"`
	Kind        NodeKind `json:"kind,omitempty"`
	Description string   `json:"description,omitempty"`

	Control  ControlPorts `json:"control"`
	DataIn   []Socket     `json:"dataIn,omitempty"`
	DataOut  []Socket     `json:"dataOut,omitempty"`
	Settings []Setting    `json:"settings,omitempty"`

	Exec ExecSpec `json:"exec"`
}

// Key is the catalog identity of this entry.
func (n NodeType) Key() string { return TypeKey(n.Type, n.TypeVersion) }

// TypeKey builds the catalog key for a type at a version.
func TypeKey(nodeType, version string) string { return nodeType + "@" + version }

// RunsOnAgent reports whether this node must be dispatched to an agent.
func (n NodeType) RunsOnAgent() bool { return n.Exec.RunsOn == RunsOnAgent }

// DataInSocket finds a declared data-in socket by name.
func (n NodeType) DataInSocket(name string) (Socket, bool) {
	return findSocket(n.DataIn, name)
}

// DataOutSocket finds a declared data-out socket by name.
func (n NodeType) DataOutSocket(name string) (Socket, bool) {
	return findSocket(n.DataOut, name)
}

func findSocket(sockets []Socket, name string) (Socket, bool) {
	for _, socket := range sockets {
		if socket.Name == name {
			return socket, true
		}
	}
	return Socket{}, false
}

// HasControlIn reports whether name is a declared control in-port.
func (n NodeType) HasControlIn(name string) bool {
	return containsString(n.Control.In, name)
}

// HasControlOut reports whether name is a declared control out-port,
// including the implicit error port.
func (n NodeType) HasControlOut(name string) bool {
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

// Validate checks a single catalog entry for the invariants the rest of the
// system assumes. A catalog that fails this is a load error rather than a
// warning: a malformed entry would otherwise surface as a confusing failure
// deep inside a run.
func (n NodeType) Validate() error {
	var problems []string

	if n.Type == "" {
		problems = append(problems, "type is required")
	} else if !strings.Contains(n.Type, "/") {
		problems = append(problems, "type must be namespaced, e.g. core/trickplay")
	}
	if n.TypeVersion == "" {
		problems = append(problems, "typeVersion is required")
	}
	if n.Name == "" {
		problems = append(problems, "name is required")
	}

	// Effects is mandatory rather than defaulted: defaulting it would mean
	// guessing whether an undeclared node writes to disk, and guessing wrong
	// in the permissive direction is how a dry run mutates a library.
	if !n.Exec.Effects.Valid() {
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

	problems = append(problems, n.portProblems()...)

	if len(problems) > 0 {
		return fmt.Errorf("node type %s: %s", n.Key(), strings.Join(problems, "; "))
	}
	return nil
}

// portProblems checks that no two ports on the same node collide. Names must
// be unique within each direction, and the reserved error port must not be
// declared by hand.
func (n NodeType) portProblems() []string {
	var problems []string

	if containsString(n.Control.Out, ErrorPort) {
		problems = append(problems, "control.out must not list "+ErrorPort+" — set control.error instead")
	}
	problems = append(problems, duplicateProblems("control.in", n.Control.In)...)
	problems = append(problems, duplicateProblems("control.out", n.Control.Out)...)

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

func findSetting(settings []Setting, name string) (Setting, bool) {
	for _, setting := range settings {
		if setting.Name == name {
			return setting, true
		}
	}
	return Setting{}, false
}

// Catalog is the loaded set of node types, keyed by type and version.
type Catalog struct {
	entries map[string]NodeType
	ordered []NodeType
}

// ErrCatalogEmpty is returned when a catalog contains no entries, which is
// almost certainly a misconfigured path rather than an intentional state.
var ErrCatalogEmpty = errors.New("workflow: catalog contains no node types")

// NewCatalog validates every entry and indexes them. A single bad entry
// fails the whole catalog, so a typo is caught at startup rather than when
// somebody happens to drag that node onto a canvas.
func NewCatalog(entries []NodeType) (*Catalog, error) {
	if len(entries) == 0 {
		return nil, ErrCatalogEmpty
	}

	indexed := make(map[string]NodeType, len(entries))
	ordered := make([]NodeType, 0, len(entries))
	for _, entry := range entries {
		if err := entry.Validate(); err != nil {
			return nil, err
		}
		if _, duplicate := indexed[entry.Key()]; duplicate {
			return nil, fmt.Errorf("workflow: catalog declares %s more than once", entry.Key())
		}
		indexed[entry.Key()] = entry
		ordered = append(ordered, entry)
	}
	return &Catalog{entries: indexed, ordered: ordered}, nil
}

// Lookup finds a node type by the identity a stored node instance carries.
func (c *Catalog) Lookup(nodeType, version string) (NodeType, bool) {
	entry, found := c.entries[TypeKey(nodeType, version)]
	return entry, found
}

// All returns every entry in catalog order, for serving to the editor.
func (c *Catalog) All() []NodeType {
	all := make([]NodeType, len(c.ordered))
	copy(all, c.ordered)
	return all
}

// Len reports how many node types are loaded.
func (c *Catalog) Len() int { return len(c.entries) }
