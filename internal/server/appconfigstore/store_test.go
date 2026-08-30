package appconfigstore

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/eventbus"
)

// fakeBackend plays both the store's dependencies: it decodes and stores
// whatever a Fire call carries, so the next Get sees it — exactly the
// property Mutate's lock exists to make safe to rely on. Get sleeps briefly
// after copying the document so an unlocked caller has room to interleave;
// without the lock, TestMutate_ConcurrentMutationsOnDifferentFieldsBothSurvive
// fails.
type fakeBackend struct {
	mu       sync.Mutex
	cfg      appconfig.Config
	getCalls int
	fired    []eventbus.Event
}

func (f *fakeBackend) Get(_ context.Context) (*appconfig.Config, error) {
	f.mu.Lock()
	f.getCalls++
	cfgCopy := f.cfg
	f.mu.Unlock()

	time.Sleep(5 * time.Millisecond)

	return &cfgCopy, nil
}

func (f *fakeBackend) Fire(_ context.Context, _ string, event eventbus.Event) error {
	var cfg appconfig.Config
	if err := json.Unmarshal(event.Payload, &cfg); err != nil {
		return err
	}

	f.mu.Lock()
	f.cfg = cfg
	f.fired = append(f.fired, event)
	f.mu.Unlock()
	return nil
}

func TestMutate_ReadsAppliesAndFiresOneEvent(t *testing.T) {
	backend := &fakeBackend{}
	store := New(backend, backend)

	err := store.Mutate(context.Background(), func(cfg *appconfig.Config) error {
		cfg.Logging.ServerLevel = appconfig.LogLevelDebug
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if backend.getCalls != 1 {
		t.Fatalf("expected exactly one read, got %d", backend.getCalls)
	}
	if len(backend.fired) != 1 {
		t.Fatalf("expected exactly one fired event, got %d", len(backend.fired))
	}
	if backend.fired[0].Name != eventbus.SystemConfigUpdateEventName {
		t.Fatalf("unexpected event name: %q", backend.fired[0].Name)
	}

	var fired appconfig.Config
	if err := json.Unmarshal(backend.fired[0].Payload, &fired); err != nil {
		t.Fatalf("fired payload did not decode: %v", err)
	}
	if fired.Logging.ServerLevel != appconfig.LogLevelDebug {
		t.Fatalf("fired document missing the change: %+v", fired)
	}
}

func TestMutate_ErrorFromApplyAbortsWithoutFiring(t *testing.T) {
	backend := &fakeBackend{}
	store := New(backend, backend)

	sentinel := errors.New("mapping rejected")
	err := store.Mutate(context.Background(), func(cfg *appconfig.Config) error {
		return sentinel
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the apply error back unchanged, got %v", err)
	}
	if len(backend.fired) != 0 {
		t.Fatalf("expected nothing fired after a rejected mutation, got %d", len(backend.fired))
	}
}

func TestMutate_CorrelationIDComesFromContext(t *testing.T) {
	backend := &fakeBackend{}
	store := New(backend, backend)

	ctx := correlation.WithID(context.Background(), "corr-123")
	err := store.Mutate(ctx, func(cfg *appconfig.Config) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(backend.fired) != 1 {
		t.Fatalf("expected one fired event, got %d", len(backend.fired))
	}
	if backend.fired[0].CorrelationID != "corr-123" {
		t.Fatalf("expected correlation id from context, got %q", backend.fired[0].CorrelationID)
	}
}

func TestMutate_ConcurrentMutationsOnDifferentFieldsBothSurvive(t *testing.T) {
	backend := &fakeBackend{}
	store := New(backend, backend)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_ = store.Mutate(context.Background(), func(cfg *appconfig.Config) error {
			cfg.Logging.ServerLevel = appconfig.LogLevelDebug
			return nil
		})
	}()
	go func() {
		defer wg.Done()
		_ = store.Mutate(context.Background(), func(cfg *appconfig.Config) error {
			cfg.Admin.Email = "second-admin@example.com"
			return nil
		})
	}()
	wg.Wait()

	final, err := store.Read(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final.Logging.ServerLevel != appconfig.LogLevelDebug {
		t.Errorf("lost the log level change: %+v", final)
	}
	if final.Admin.Email != "second-admin@example.com" {
		t.Errorf("lost the admin email change: %+v", final)
	}
}

func TestRead_DelegatesToReader(t *testing.T) {
	backend := &fakeBackend{cfg: appconfig.Config{Admin: appconfig.AdminUser{Username: "admin"}}}
	store := New(backend, backend)

	cfg, err := store.Read(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Admin.Username != "admin" {
		t.Fatalf("expected the stored document, got %+v", cfg)
	}
}
