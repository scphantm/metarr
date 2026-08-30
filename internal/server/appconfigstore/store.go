// Package appconfigstore is the single path through which the application
// config changes. Every mutation reads the current document, applies one
// named change, and fires it as a system_config_update event — all under a
// lock, so two changes to different settings computed around the same time
// can no longer revert one another. See docs/adr/0001 for why a client may
// no longer supply a whole document, and docs/adr/0002 for why the write
// stays asynchronous and the lock is process-local.
//
// Bootstrap and SeedAdmin are the exception: startup seeding runs before any
// listener exists to persist a fired event, so it writes synchronously
// through the same lock instead. See docs/adr/0003 for why that needs a
// different contract than Mutate's rather than reusing it.
package appconfigstore

import (
	"context"
	"sync"
	"time"

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

// updateFirer is the store's write dependency, satisfied by
// *eventbus.StreamBus without any change to that type.
type updateFirer interface {
	Fire(ctx context.Context, stream string, event eventbus.Event) error
}

// configWriter is Bootstrap's persistence dependency — a direct, synchronous
// write, unlike Mutate's event-firing one. Satisfied by
// *mongostore.AppConfigRepo without any change to that type.
type configWriter interface {
	Upsert(ctx context.Context, cfg *appconfig.Config) error
}

// Store is the config store.
type Store struct {
	mu     sync.Mutex
	reader configReader
	writer configWriter
	firer  updateFirer
}

// New returns a config store backed by reader, writer, and firer.
func New(reader configReader, writer configWriter, firer updateFirer) *Store {
	return &Store{reader: reader, writer: writer, firer: firer}
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

	event := eventbus.Event{
		CorrelationID: correlation.FromContext(ctx),
		Name:          eventbus.SystemConfigUpdateEventName,
		Payload:       payload,
		Timestamp:     time.Now().UTC(),
	}

	return s.firer.Fire(ctx, eventbus.SystemConfigUpdateStream, event)
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
