package workflow

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"path/filepath"
)

// DryRunFS wraps fsys so that every mutating call is recorded and discarded
// instead of reaching the disk. Reads pass straight through, because a
// simulated run still has to inspect real files to produce plausible results.
//
// This is the enforcement point for dry-run. It is a wrapper rather than a
// flag checked inside each operation so that the suppression cannot be
// bypassed by a handler that forgets: the handler is holding this object, and
// there is no other filesystem to reach.
func DryRunFS(fsys WorkFS, log *slog.Logger) WorkFS {
	return &dryRunFS{inner: fsys, log: log}
}

type dryRunFS struct {
	inner WorkFS
	log   *slog.Logger
}

// simulated records an intended mutation. Every suppressed effect is logged
// even in dry-run — that is what makes "what would this actually have done?"
// answerable without running it for real.
func (d *dryRunFS) simulated(operation string, attributes ...any) {
	d.log.Info("filesystem effect simulated",
		append([]any{"operation", operation, "dry_run", true}, attributes...)...)
}

func (d *dryRunFS) Open(name string) (io.ReadCloser, error) { return d.inner.Open(name) }
func (d *dryRunFS) Stat(name string) (fs.FileInfo, error)   { return d.inner.Stat(name) }
func (d *dryRunFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return d.inner.ReadDir(name)
}

func (d *dryRunFS) MkdirAll(path string) error {
	d.simulated("mkdir_all", "path", path)
	return nil
}

func (d *dryRunFS) WriteFile(name string, data []byte) error {
	d.simulated("write_file", "path", name, "bytes", len(data))
	return nil
}

func (d *dryRunFS) Create(name string) (io.WriteCloser, error) {
	d.simulated("create", "path", name)
	return discardWriteCloser{}, nil
}

func (d *dryRunFS) Rename(oldPath, newPath string) error {
	d.simulated("rename", "from", oldPath, "to", newPath)
	return nil
}

func (d *dryRunFS) Remove(name string) error {
	d.simulated("remove", "path", name)
	return nil
}

func (d *dryRunFS) RemoveAll(path string) error {
	d.simulated("remove_all", "path", path)
	return nil
}

func (d *dryRunFS) TempFileIn(destination string) (string, io.WriteCloser, error) {
	// The path handed back is plausible rather than real, so a handler that
	// goes on to log or return it produces the same shape it would in a live
	// run. Nothing is created.
	simulatedPath := filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+".dryrun")
	d.simulated("temp_file", "path", simulatedPath, "destination", destination)
	return simulatedPath, discardWriteCloser{}, nil
}

type discardWriteCloser struct{}

func (discardWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (discardWriteCloser) Close() error                { return nil }

// DryRunRunner wraps runner so that invocations marked Mutates are recorded
// and skipped. Read-only invocations still run: a simulated transcode has to
// probe the real file to report what it would have produced.
func DryRunRunner(runner Runner, log *slog.Logger) Runner {
	return &dryRunRunner{inner: runner, log: log}
}

type dryRunRunner struct {
	inner Runner
	log   *slog.Logger
}

func (d *dryRunRunner) Run(ctx context.Context, command Command) (CommandResult, error) {
	if !command.Mutates {
		return d.inner.Run(ctx, command)
	}
	d.log.Info("command simulated",
		"command", command.Path,
		"args", command.Args,
		"dir", command.Dir,
		"dry_run", true,
	)
	return CommandResult{Simulated: true}, nil
}
