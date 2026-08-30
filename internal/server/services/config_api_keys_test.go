package services

import (
	"context"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/appconfigstore"
	"Metarr/internal/server/handlers"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/eventbus"
)

// fakeConfigBackend satisfies appconfigstore's Get/Upsert/Fire dependencies,
// persisting whatever a Fire or Upsert call carries so the next Get sees it
// — enough to drive a real ConfigServer end to end with no MongoDB or
// Redis. The config types are proto messages, so the document is held and
// handed out as a clone rather than aliased across the seam.
type fakeConfigBackend struct {
	cfg   *appconfig.Config
	fired []eventbus.Event
}

func (f *fakeConfigBackend) Get(_ context.Context) (*appconfig.Config, error) {
	if f.cfg == nil {
		return &appconfig.Config{}, nil
	}
	return proto.Clone(f.cfg).(*appconfig.Config), nil
}

func (f *fakeConfigBackend) Fire(_ context.Context, _ string, event eventbus.Event) error {
	cfg, err := appconfig.UnmarshalStored(event.Payload)
	if err != nil {
		return err
	}
	f.cfg = cfg
	f.fired = append(f.fired, event)
	return nil
}

func (f *fakeConfigBackend) Upsert(_ context.Context, cfg *appconfig.Config) error {
	f.cfg = proto.Clone(cfg).(*appconfig.Config)
	return nil
}

func newTestConfigServer(seed *appconfig.Config) (*ConfigServer, *fakeConfigBackend) {
	backend := &fakeConfigBackend{cfg: seed}
	store := appconfigstore.New(backend, backend, backend)
	return &ConfigServer{Handlers: &handlers.Handlers{
		AppConfigStore: store,
		Logger:         slog.Default(),
	}}, backend
}

func TestUpsertApiKey_LeavesAdminCredentialsByteIdentical(t *testing.T) {
	seed := &appconfig.Config{
		Admin: &appconfig.AdminUser{
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
	server, backend := newTestConfigServer(&appconfig.Config{})

	_, err := server.UpsertApiKey(context.Background(), connect.NewRequest(&metarrv1.ConfigServiceUpsertApiKeyRequest{
		Group: "admin",
		Entry: &metarrv1.APIKeyEntry{Name: "new key", ApiKey: "secret-value"},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(backend.cfg.ApiKeys.Admin) != 1 {
		t.Fatalf("expected one entry, got %d", len(backend.cfg.ApiKeys.Admin))
	}
	entry := backend.cfg.ApiKeys.Admin[0]
	if entry.Name != "new key" || entry.ApiKey != "secret-value" {
		t.Fatalf("entry not stored correctly: %+v", entry)
	}
	if entry.Id == "" {
		t.Fatal("expected a minted id")
	}
}

func TestUpsertApiKey_ReplacesAnExistingEntryByID(t *testing.T) {
	seed := &appconfig.Config{
		ApiKeys: &appconfig.APIKeysConfig{
			User: []*appconfig.APIKeyEntry{{Id: "existing-id", Name: "old-name", ApiKey: "old-key"}},
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

	if len(backend.cfg.ApiKeys.User) != 1 {
		t.Fatalf("expected the group to stay at 1 entry, got %d", len(backend.cfg.ApiKeys.User))
	}
	if backend.cfg.ApiKeys.User[0].Name != "new-name" || backend.cfg.ApiKeys.User[0].ApiKey != "new-key" {
		t.Fatalf("entry was not replaced: %+v", backend.cfg.ApiKeys.User[0])
	}
}

func TestUpsertApiKey_RejectsAnUnknownID(t *testing.T) {
	seed := &appconfig.Config{
		ApiKeys: &appconfig.APIKeysConfig{
			Admin: []*appconfig.APIKeyEntry{{Id: "still-here", Name: "unrelated"}},
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
	if len(backend.cfg.ApiKeys.Admin) != 1 || backend.cfg.ApiKeys.Admin[0].Id != "still-here" {
		t.Fatalf("group must be untouched after a rejected upsert: %+v", backend.cfg.ApiKeys.Admin)
	}
}

func TestUpsertApiKey_RejectsAnUnknownGroup(t *testing.T) {
	server, _ := newTestConfigServer(&appconfig.Config{})

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
	seed := &appconfig.Config{
		ApiKeys: &appconfig.APIKeysConfig{
			Webhook: []*appconfig.APIKeyEntry{
				{Id: "keep", Name: "keep-me"},
				{Id: "remove", Name: "remove-me"},
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

	if len(backend.cfg.ApiKeys.Webhook) != 1 || backend.cfg.ApiKeys.Webhook[0].Id != "keep" {
		t.Fatalf("unexpected state after delete: %+v", backend.cfg.ApiKeys.Webhook)
	}
}

func TestDeleteApiKey_ReportsNotFoundForAnUnknownID(t *testing.T) {
	server, backend := newTestConfigServer(&appconfig.Config{
		ApiKeys: &appconfig.APIKeysConfig{ReadOnly: []*appconfig.APIKeyEntry{{Id: "a"}}},
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
	if len(backend.cfg.ApiKeys.ReadOnly) != 1 {
		t.Fatalf("group must be untouched after a rejected delete: %+v", backend.cfg.ApiKeys.ReadOnly)
	}
}

func TestDeleteApiKey_LeavesOtherGroupsUntouched(t *testing.T) {
	seed := &appconfig.Config{
		ApiKeys: &appconfig.APIKeysConfig{
			Admin: []*appconfig.APIKeyEntry{{Id: "a", Name: "admin-key"}},
			User:  []*appconfig.APIKeyEntry{{Id: "u", Name: "user-key"}},
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

	if len(backend.cfg.ApiKeys.User) != 1 || backend.cfg.ApiKeys.User[0].Name != "user-key" {
		t.Fatalf("user group was disturbed: %+v", backend.cfg.ApiKeys.User)
	}
}
