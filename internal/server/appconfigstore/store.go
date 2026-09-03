// Package appconfigstore is the single path through which the application
// config changes. Every mutation reads the current document, applies one
// named change, and writes the result — all under a lock, so two changes to
// different settings computed around the same time can no longer revert one
// another. See docs/adr/0001 for why a client may no longer supply a whole
// document, and docs/adr/0002 for why the lock is process-local.
//
// MutateSync is the one config-mutation path: it reads, applies, persists
// directly to Mongo, and propagates the result in-process — all under the
// lock — before it returns, so the calling RPC can hand back the stored
// resource (docs/adr/0002). Bootstrap writes synchronously too but only when
// apply reports a change, and it does not propagate: startup seeding runs
// before the rest of the process is up (docs/adr/0003).
package appconfigstore

import (
	"context"
	"sync"

	"Metarr/internal/shared/appconfig"
)

// configReader is the store's read dependency. Declared here, at the
// consumer, so it stays narrow — it is satisfied by
// *mongostore.AppConfigRepo without any change to that type.
type configReader interface {
	Get(ctx context.Context) (*appconfig.Config, error)
}

// configWriter is the store's direct persistence dependency — a synchronous
// Mongo write, used by Bootstrap and by MutateSync. Satisfied by
// *mongostore.AppConfigRepo without any change to that type.
type configWriter interface {
	Upsert(ctx context.Context, cfg *appconfig.Config) error
}

// inProcessPropagator applies an already-persisted config to the rest of the
// process — swap the live-config singleton, set the server log level,
// recompile the sidecar registry, republish agent projections. MutateSync
// calls it after its Mongo write. Satisfied by *listeners.ConfigPropagator;
// declared here, at the consumer, so appconfigstore does not import
// listeners. A propagation failure is the propagator's to log, never
// returned — the write has already landed (docs/adr/0002).
type inProcessPropagator interface {
	PropagateInProcess(ctx context.Context, cfg *appconfig.Config) error
}

// Store is the config store.
type Store struct {
	mu         sync.Mutex
	reader     configReader
	writer     configWriter
	propagator inProcessPropagator
}

// New returns a config store backed by reader and writer. Call SetPropagator
// before the first MutateSync to wire the synchronous write path's in-process
// propagation.
func New(reader configReader, writer configWriter) *Store {
	return &Store{reader: reader, writer: writer}
}

// SetPropagator wires the in-process propagation MutateSync runs after its
// Mongo write. It is set once at startup, before the server serves, so it
// needs no lock of its own.
func (s *Store) SetPropagator(p inProcessPropagator) {
	s.propagator = p
}

// Read returns the currently stored application config, straight from
// storage. It exists for startup bootstrap, before live config exists to
// read instead — general server code wants appconfig.Get(), not this.
func (s *Store) Read(ctx context.Context) (*appconfig.Config, error) {
	cfg, err := s.reader.Get(ctx)
	if err != nil {
		return nil, err
	}
	return appconfig.Normalize(cfg), nil
}

// MutateSync reads the current application config, applies apply to it,
// persists the result directly to storage, and then propagates it in-process
// — all while holding the store's lock, so the whole read-modify-write-
// propagate sequence is serialized against any concurrent MutateSync. The
// write has already landed and been made live by the time this returns, so a
// caller's RPC can return the stored resource (docs/adr/0002).
//
// An error from apply aborts before anything is written and is returned
// unchanged. A storage write failure is returned too, with live config left
// untouched. A propagation failure after a successful write is logged by the
// propagator, never returned — redoing the write to retry propagation would
// be worse than the transient miss.
func (s *Store) MutateSync(ctx context.Context, apply func(*appconfig.Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.reader.Get(ctx)
	if err != nil {
		return err
	}
	cfg = appconfig.Normalize(cfg)

	if err := apply(cfg); err != nil {
		return err
	}

	if err := s.writer.Upsert(ctx, cfg); err != nil {
		return err
	}

	if s.propagator != nil {
		_ = s.propagator.PropagateInProcess(ctx, cfg)
	}
	return nil
}

// Bootstrap reads the current application config, applies apply to it, and
// — only if apply reports a change — persists the result directly, under
// the same lock MutateSync uses. Unlike MutateSync it does not propagate:
// it exists for startup seeding, which runs before the rest of the process
// is up. An ordinary restart where apply changes nothing costs no write.
// See docs/adr/0003.
func (s *Store) Bootstrap(ctx context.Context, apply func(*appconfig.Config) (changed bool, err error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.reader.Get(ctx)
	if err != nil {
		return err
	}
	cfg = appconfig.Normalize(cfg)

	changed, err := apply(cfg)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	return s.writer.Upsert(ctx, cfg)
}
