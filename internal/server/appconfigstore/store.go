// Package appconfigstore is the single path through which the application
// config changes. Every mutation reads the current document, applies one
// named change, and fires it as a system_config_update event — all under a
// lock, so two changes to different settings computed around the same time
// can no longer revert one another. See docs/adr/0001 for why a client may
// no longer supply a whole document, and docs/adr/0002 for why the write
// stays asynchronous and the lock is process-local.
package appconfigstore

import (
	"context"
	"encoding/json"
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

// Store is the config store.
type Store struct {
	mu     sync.Mutex
	reader configReader
	firer  updateFirer
}

// New returns a config store backed by reader and firer.
func New(reader configReader, firer updateFirer) *Store {
	return &Store{reader: reader, firer: firer}
}

// Read returns the currently stored application config.
func (s *Store) Read(ctx context.Context) (*appconfig.Config, error) {
	return s.reader.Get(ctx)
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

	if err := apply(cfg); err != nil {
		return err
	}

	payload, err := json.Marshal(cfg)
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
