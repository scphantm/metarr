// Package appconfigstore is the single path through which the application
// config changes. Every mutation reads the current document, applies one
// named change, and writes the result — all under a lock, so two changes to
// different settings computed around the same time can no longer revert one
// another. See docs/adr/0001 for why a client may no longer supply a whole
// document, and docs/adr/0002 for why the lock is process-local.
//
// Two write paths exist during the AIP config-CRUD conversion. MutateSync
// persists directly and propagates in-process before it returns — the shape
// every config write is converging on (docs/adr/0002). Mutate still fires a
// system_config_update event a background listener persists, for the config
// services not yet reshaped. Bootstrap writes synchronously and fires
// nothing: startup seeding runs before any listener exists (docs/adr/0003).
package appconfigstore

import (
	"context"
	"sync"

	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/eventbus"
)

// configReader is the store's read dependency. Declared here, at the
// consumer, so it stays narrow — it is satisfied by
// *mongostore.AppConfigRepo without any change to that type.
type configReader interface {
	Get(ctx context.Context) (*appconfig.Config, error)
}

// updatePublisher is the store's write dependency, satisfied by
// *eventbus.Bus without any change to that type. The Bus stamps the envelope
// Source and validates the event name against the topic row, so the store
// hands it only name, correlation ID, and the encoded payload.
type updatePublisher interface {
	Publish(ctx context.Context, topic eventbus.StreamTopic, name, correlationID string, payload []byte) error
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
	publisher  updatePublisher
	propagator inProcessPropagator
}

// New returns a config store backed by reader, writer, and publisher. Call
// SetPropagator before the first MutateSync to wire the synchronous write
// path's in-process propagation.
func New(reader configReader, writer configWriter, publisher updatePublisher) *Store {
	return &Store{reader: reader, writer: writer, publisher: publisher}
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

// Mutate reads the current application config, applies apply to it, and
// fires the result as a system_config_update event — all while holding the
// store's lock, so a concurrent Mutate cannot start its own read until this
// one has fired. An error from apply aborts before anything is fired and is
// returned to the caller unchanged: Mutate adds no status, wraps nothing,
// and imports no transport package, so a caller's own error mapping keeps
// working exactly as it does today.
func (s *Store) Mutate(ctx context.Context, apply func(*appconfig.Config) error) error {
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

	payload, err := appconfig.MarshalStored(cfg)
	if err != nil {
		return err
	}

	return s.publisher.Publish(
		ctx,
		eventbus.SystemConfigUpdateTopic(),
		eventbus.SystemConfigUpdateEventName,
		correlation.FromContext(ctx),
		payload,
	)
}

// MutateSync reads the current application config, applies apply to it,
// persists the result directly to storage, and then propagates it in-process
// — all while holding the store's lock, so the whole read-modify-write-
// propagate sequence is serialized against any concurrent Mutate / MutateSync.
// Unlike Mutate it does not fire a system_config_update event: the write has
// already landed and been made live by the time this returns, so a caller's
// RPC can return the stored resource (docs/adr/0002).
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
// the same lock Mutate uses. Unlike Mutate it writes synchronously and fires
// no event: it exists for startup seeding, which runs before any listener
// is available to persist a fired one. An ordinary restart where apply
// changes nothing costs no write. See docs/adr/0003.
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
