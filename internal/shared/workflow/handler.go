package workflow

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
)

// Effects classifies what a node type does to the filesystem. Every catalog
// entry must declare it — it is what dry-run keys off, and it cannot be
// retrofitted without re-auditing every handler ever written.
type Effects string

const (
	// EffectsRead inspects only, and is unaffected by dry-run.
	EffectsRead Effects = "read"
	// EffectsWrite creates or modifies files.
	EffectsWrite Effects = "write"
	// EffectsDestructive deletes or overwrites existing library content. It
	// is badged in the editor, and an agent refuses to invoke it at all
	// while dry-run is set.
	EffectsDestructive Effects = "destructive"
)

// Valid reports whether e is one of the three known classifications.
func (e Effects) Valid() bool {
	return e == EffectsRead || e == EffectsWrite || e == EffectsDestructive
}

// Mutates reports whether nodes of this class can change what is on disk.
func (e Effects) Mutates() bool {
	return e == EffectsWrite || e == EffectsDestructive
}

// RunMode separates a rehearsal in the editor from a real scheduled run. It
// is deliberately independent of DryRun: a production run may still be a dry
// run, and that has to stay expressible.
type RunMode string

const (
	ModeDevelopment RunMode = "development"
	ModeProduction  RunMode = "production"
)

// WorkFS is the only filesystem a node handler is given.
//
// This is the whole dry-run guarantee: a handler never imports os, so a
// handler that forgets to check DryRun still cannot write, because it has no
// path to the filesystem to write through. Reads always pass through; every
// mutating call is suppressed and logged when dry-run is set.
//
// Never hand a handler direct filesystem access, however convenient — one
// exception and the guarantee is gone for the whole system, because the
// guarantee is exactly that there is no other path.
type WorkFS interface {
	Open(name string) (io.ReadCloser, error)
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)

	MkdirAll(path string) error
	WriteFile(name string, data []byte) error
	Create(name string) (io.WriteCloser, error)
	Rename(oldPath, newPath string) error
	Remove(name string) error
	RemoveAll(path string) error

	// TempFileIn creates a temporary file in the same directory as
	// destination and returns its path.
	//
	// It exists so the atomic-publish rule cannot be got wrong: os.Rename is
	// only atomic within a filesystem, and a cross-device rename fails
	// outright on Linux, so staging in the node's WorkDir and renaming into
	// the library would either fail or silently degrade to a non-atomic
	// copy. Write here, then Rename over the destination.
	TempFileIn(destination string) (path string, file io.WriteCloser, err error)
}

// Command is a process invocation requested by a handler.
type Command struct {
	Path string
	Args []string
	// Dir defaults to the node's WorkDir when empty.
	Dir string
	// Mutates marks an invocation that changes files on disk, so it is
	// suppressed under dry-run. It is per-invocation rather than per-node
	// because a write-effect node legitimately runs read-only tools first —
	// probing before transcoding — and those must still run during a dry run
	// or the simulated outputs would be worthless.
	Mutates bool
}

// CommandResult is the outcome of a process invocation.
type CommandResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	// Simulated is true when dry-run suppressed the invocation.
	Simulated bool
}

// Runner spawns processes on the handler's behalf. Like WorkFS, it exists so
// that handlers never import os/exec and therefore cannot escape the dry-run
// gate.
//
// Implementations must start the process in its own process group and kill
// the group on cancellation: ffmpeg spawns children, and killing only the
// parent leaves them running. This is a silent leak on every supported OS
// otherwise.
type Runner interface {
	Run(ctx context.Context, command Command) (CommandResult, error)
}

// NodeContext is everything a handler is given about the execution it is
// part of. Handlers receive their capabilities here rather than reaching for
// package-level state, which is what makes them testable and what makes the
// dry-run gate total.
type NodeContext struct {
	RunID    string
	NodeID   string
	NodeType string
	// Frame identifies the loop iteration and parallel branch this execution
	// belongs to, e.g. "/" or "/n7#3/n12#0".
	Frame   string
	Attempt int

	Mode   RunMode
	DryRun bool
	// WorkDir is local scratch space for genuine intermediates — probe
	// output, extracted artwork, multi-pass logs. It is NOT the staging area
	// for an atomic publish; use WorkFS.TempFileIn for that.
	WorkDir string

	FS   WorkFS
	Exec Runner
	// Log already carries the run's mandatory fields and honours the run's
	// own level override, so a handler just logs.
	Log *slog.Logger
}

// Inputs are the resolved data-in values for one node execution, keyed by
// socket name, already coerced through any edge transform.
type Inputs map[string]any

// Outputs are what a handler produces.
type Outputs struct {
	// ControlPort names which control-out edge to follow. Empty means the
	// node's single default out-port. A conditional returns "yes" or "no";
	// this is how branching is expressed.
	ControlPort string
	// Data holds the data-out values, keyed by socket name. A write-effect
	// handler must populate these even under dry-run — returning the path it
	// would have written — or nothing downstream can be simulated.
	Data map[string]any
}

// Handler executes one node type.
//
// The interface is shared by both binaries, but the registries are not: the
// server's and the agent's handler sets are disjoint, and a shared registry
// is exactly how a server import would sneak into the agent's build graph.
type Handler interface {
	Execute(ctx context.Context, node NodeContext, in Inputs) (Outputs, error)
}
