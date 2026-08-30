package services

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/appconfigstore"
	"Metarr/internal/server/handlers"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/eventbus"
)

// fakeConfigBackend satisfies appconfigstore's Get/Fire dependencies,
// persisting whatever a Fire call carries so the next Get sees it — enough
// to drive a real ConfigServer end to end with no MongoDB or Redis.
type fakeConfigBackend struct {
	cfg   appconfig.Config
	fired []eventbus.Event
}

func (f *fakeConfigBackend) Get(_ context.Context) (*appconfig.Config, error) {
	cfgCopy := f.cfg
	return &cfgCopy, nil
}

func (f *fakeConfigBackend) Fire(_ context.Context, _ string, event eventbus.Event) error {
	var cfg appconfig.Config
	if err := json.Unmarshal(event.Payload, &cfg); err != nil {
		return err
	}
	f.cfg = cfg
	f.fired = append(f.fired, event)
	return nil
}

func newTestConfigServer(seed appconfig.Config) (*ConfigServer, *fakeConfigBackend) {
	backend := &fakeConfigBackend{cfg: seed}
	store := appconfigstore.New(backend, backend)
	return &ConfigServer{Handlers: &handlers.Handlers{
		AppConfigStore: store,
		Logger:         slog.Default(),
	}}, backend
}

func TestUpsertApiKey_LeavesAdminCredentialsByteIdentical(t *testing.T) {
	seed := appconfig.Config{
		Admin: appconfig.AdminUser{
			Username:     "admin",
			Email:        "admin@example.com",
			PasswordSalt: "original-salt",
			PasswordHash: "original-hash",
		},
	}
	server, backend := newTestConfigServer(seed)

	_, err := server.UpsertApiKey(context.Background(), connect.NewRequest(&metarrv1.ConfigServiceUpsertApiKeyRequest{
		Group: "admin",
		Entry: &metarrv1.APIKeyEntry{Name: "new key", ApiKey: "secret-value"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if backend.cfg.Admin.PasswordSalt != "original-salt" || backend.cfg.Admin.PasswordHash != "original-hash" {
		t.Fatalf("admin credentials were disturbed: %+v", backend.cfg.Admin)
	}
	if backend.cfg.Admin.Username != "admin" || backend.cfg.Admin.Email != "admin@example.com" {
		t.Fatalf("admin identity was disturbed: %+v", backend.cfg.Admin)
	}
}

func TestUpsertApiKey_CreatesAnEntryWithAMintedID(t *testing.T) {
	server, backend := newTestConfigServer(appconfig.Config{})

	_, err := server.UpsertApiKey(context.Background(), connect.NewRequest(&metarrv1.ConfigServiceUpsertApiKeyRequest{
		Group: "admin",
		Entry: &metarrv1.APIKeyEntry{Name: "new key", ApiKey: "secret-value"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(backend.cfg.APIKeys.Admin) != 1 {
		t.Fatalf("expected one entry, got %d", len(backend.cfg.APIKeys.Admin))
	}
	entry := backend.cfg.APIKeys.Admin[0]
	if entry.Name != "new key" || entry.Key != "secret-value" {
		t.Fatalf("entry not stored correctly: %+v", entry)
	}
	if entry.ID == "" {
		t.Fatal("expected a minted id")
	}
}

func TestUpsertApiKey_ReplacesAnExistingEntryByID(t *testing.T) {
	seed := appconfig.Config{
		APIKeys: appconfig.APIKeysConfig{
			User: []appconfig.APIKeyEntry{{ID: "existing-id", Name: "old-name", Key: "old-key"}},
		},
	}
	server, backend := newTestConfigServer(seed)

	_, err := server.UpsertApiKey(context.Background(), connect.NewRequest(&metarrv1.ConfigServiceUpsertApiKeyRequest{
		Group: "user",
		Entry: &metarrv1.APIKeyEntry{Id: "existing-id", Name: "new-name", ApiKey: "new-key"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(backend.cfg.APIKeys.User) != 1 {
		t.Fatalf("expected the group to stay at 1 entry, got %d", len(backend.cfg.APIKeys.User))
	}
	if backend.cfg.APIKeys.User[0].Name != "new-name" || backend.cfg.APIKeys.User[0].Key != "new-key" {
		t.Fatalf("entry was not replaced: %+v", backend.cfg.APIKeys.User[0])
	}
}

func TestUpsertApiKey_RejectsAnUnknownID(t *testing.T) {
	seed := appconfig.Config{
		APIKeys: appconfig.APIKeysConfig{
			Admin: []appconfig.APIKeyEntry{{ID: "still-here", Name: "unrelated"}},
		},
	}
	server, backend := newTestConfigServer(seed)

	// The reported scenario: a client holds a stale, non-empty id for an
	// entry deleted out from under it. Without this check, UpsertApiKey's
	// underlying appconfig.UpsertAPIKey appends rather than replaces on an
	// unknown id, resurrecting the deleted entry instead of failing.
	_, err := server.UpsertApiKey(context.Background(), connect.NewRequest(&metarrv1.ConfigServiceUpsertApiKeyRequest{
		Group: "admin",
		Entry: &metarrv1.APIKeyEntry{Id: "deleted-out-from-under-the-client", Name: "resurrected?"},
	}))
	if err == nil {
		t.Fatal("expected an error for an unknown id")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("expected NotFound, got %v", connect.CodeOf(err))
	}
	if len(backend.fired) != 0 {
		t.Fatalf("a rejected upsert must not fire anything, got %d fired events", len(backend.fired))
	}
	if len(backend.cfg.APIKeys.Admin) != 1 || backend.cfg.APIKeys.Admin[0].ID != "still-here" {
		t.Fatalf("group must be untouched after a rejected upsert: %+v", backend.cfg.APIKeys.Admin)
	}
}

func TestUpsertApiKey_RejectsAnUnknownGroup(t *testing.T) {
	server, _ := newTestConfigServer(appconfig.Config{})

	_, err := server.UpsertApiKey(context.Background(), connect.NewRequest(&metarrv1.ConfigServiceUpsertApiKeyRequest{
		Group: "bogus",
		Entry: &metarrv1.APIKeyEntry{Name: "x"},
	}))
	if err == nil {
		t.Fatal("expected an error for an unknown group")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestDeleteApiKey_RemovesExactlyOneEntry(t *testing.T) {
	seed := appconfig.Config{
		APIKeys: appconfig.APIKeysConfig{
			Webhook: []appconfig.APIKeyEntry{
				{ID: "keep", Name: "keep-me"},
				{ID: "remove", Name: "remove-me"},
			},
		},
	}
	server, backend := newTestConfigServer(seed)

	_, err := server.DeleteApiKey(context.Background(), connect.NewRequest(&metarrv1.ConfigServiceDeleteApiKeyRequest{
		Group: "webhook",
		Id:    "remove",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(backend.cfg.APIKeys.Webhook) != 1 || backend.cfg.APIKeys.Webhook[0].ID != "keep" {
		t.Fatalf("unexpected state after delete: %+v", backend.cfg.APIKeys.Webhook)
	}
}

func TestDeleteApiKey_ReportsNotFoundForAnUnknownID(t *testing.T) {
	server, backend := newTestConfigServer(appconfig.Config{
		APIKeys: appconfig.APIKeysConfig{ReadOnly: []appconfig.APIKeyEntry{{ID: "a"}}},
	})

	_, err := server.DeleteApiKey(context.Background(), connect.NewRequest(&metarrv1.ConfigServiceDeleteApiKeyRequest{
		Group: "read_only",
		Id:    "unknown",
	}))
	if err == nil {
		t.Fatal("expected an error for an unknown id")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("expected NotFound, got %v", connect.CodeOf(err))
	}
	if len(backend.fired) != 0 {
		t.Fatalf("a rejected delete must not fire anything, got %d fired events", len(backend.fired))
	}
	if len(backend.cfg.APIKeys.ReadOnly) != 1 {
		t.Fatalf("group must be untouched after a rejected delete: %+v", backend.cfg.APIKeys.ReadOnly)
	}
}

func TestDeleteApiKey_LeavesOtherGroupsUntouched(t *testing.T) {
	seed := appconfig.Config{
		APIKeys: appconfig.APIKeysConfig{
			Admin: []appconfig.APIKeyEntry{{ID: "a", Name: "admin-key"}},
			User:  []appconfig.APIKeyEntry{{ID: "u", Name: "user-key"}},
		},
	}
	server, backend := newTestConfigServer(seed)

	_, err := server.DeleteApiKey(context.Background(), connect.NewRequest(&metarrv1.ConfigServiceDeleteApiKeyRequest{
		Group: "admin",
		Id:    "a",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(backend.cfg.APIKeys.User) != 1 || backend.cfg.APIKeys.User[0].Name != "user-key" {
		t.Fatalf("user group was disturbed: %+v", backend.cfg.APIKeys.User)
	}
}
