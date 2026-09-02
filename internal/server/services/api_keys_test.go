package services

import (
	"context"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/appconfigstore"
	"Metarr/internal/server/handlers"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

func newTestApiKeyServer(seed *appconfig.Config) (*ApiKeyServer, *fakeConfigBackend) {
	backend := &fakeConfigBackend{cfg: seed}
	store := appconfigstore.New(backend, backend, backend)
	return &ApiKeyServer{Handlers: &handlers.Handlers{
		AppConfigStore: store,
		Logger:         slog.Default(),
	}}, backend
}

func createKeyReq(level metarrv1.AccessLevel, entry *metarrv1.APIKeyEntry) *connect.Request[metarrv1.CreateApiKeyRequest] {
	return connect.NewRequest(&metarrv1.CreateApiKeyRequest{AccessLevel: level, ApiKey: entry})
}

func TestCreateApiKey_ReturnsTheResourceWithAMintedID(t *testing.T) {
	server, backend := newTestApiKeyServer(&appconfig.Config{
		Admin: &appconfig.AdminUser{Username: "admin", PasswordHash: "keep-me"},
	})

	resp, err := server.CreateApiKey(correlation.WithID(context.Background(), "c1"),
		createKeyReq(metarrv1.AccessLevel_ACCESS_LEVEL_WEBHOOK, &metarrv1.APIKeyEntry{Name: "inbound", ApiKey: "v"}))
	if err != nil {
		t.Fatalf("CreateApiKey: %v", err)
	}
	if resp.Msg.GetId() == "" {
		t.Fatal("response carried no minted id")
	}
	stored := backend.cfg.GetApiKeys().GetWebhook()
	if len(stored) != 1 || stored[0].GetId() != resp.Msg.GetId() || stored[0].GetName() != "inbound" {
		t.Fatalf("stored webhook keys = %+v", stored)
	}
	if backend.cfg.GetAdmin().GetPasswordHash() != "keep-me" {
		t.Fatalf("an API-key write disturbed the admin credential: %+v", backend.cfg.GetAdmin())
	}
	if len(backend.fired) != 0 {
		t.Fatalf("a synchronous write fired %d events, want 0", len(backend.fired))
	}
}

func TestCreateApiKey_RejectsAClientSuppliedID(t *testing.T) {
	server, _ := newTestApiKeyServer(&appconfig.Config{})

	_, err := server.CreateApiKey(context.Background(),
		createKeyReq(metarrv1.AccessLevel_ACCESS_LEVEL_USER, &metarrv1.APIKeyEntry{Id: "mine", Name: "x"}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestCreateApiKey_UnspecifiedAccessLevelIsInvalidArgument(t *testing.T) {
	server, _ := newTestApiKeyServer(&appconfig.Config{})

	_, err := server.CreateApiKey(context.Background(),
		createKeyReq(metarrv1.AccessLevel_ACCESS_LEVEL_UNSPECIFIED, &metarrv1.APIKeyEntry{Name: "x"}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestListApiKeys_ReturnsOneLevelsEntries(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{ApiKeys: &appconfig.APIKeysConfig{
		Admin: []*appconfig.APIKeyEntry{{Id: "a1", Name: "admin-one"}},
		User:  []*appconfig.APIKeyEntry{{Id: "u1", Name: "user-one"}, {Id: "u2", Name: "user-two"}},
	}})
	server, _ := newTestApiKeyServer(nil)

	resp, err := server.ListApiKeys(context.Background(), connect.NewRequest(&metarrv1.ListApiKeysRequest{
		AccessLevel: metarrv1.AccessLevel_ACCESS_LEVEL_USER,
		OrderBy:     "name",
	}))
	if err != nil {
		t.Fatalf("ListApiKeys: %v", err)
	}
	if len(resp.Msg.GetApiKeys()) != 2 {
		t.Fatalf("got %d entries, want the 2 user-level ones", len(resp.Msg.GetApiKeys()))
	}
	if resp.Msg.GetApiKeys()[0].GetName() != "user-one" || resp.Msg.GetApiKeys()[1].GetName() != "user-two" {
		t.Fatalf("order_by name not honoured: %+v", resp.Msg.GetApiKeys())
	}
}

func TestListApiKeys_UnspecifiedAccessLevelIsInvalidArgument(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{})
	server, _ := newTestApiKeyServer(nil)

	_, err := server.ListApiKeys(context.Background(), connect.NewRequest(&metarrv1.ListApiKeysRequest{}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestGetApiKey_FoundAcrossGroupsAndNotFound(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{ApiKeys: &appconfig.APIKeysConfig{
		ReadOnly: []*appconfig.APIKeyEntry{{Id: "r1", Name: "ro", ApiKey: "secret"}},
	}})
	server, _ := newTestApiKeyServer(nil)

	resp, err := server.GetApiKey(context.Background(), connect.NewRequest(&metarrv1.GetApiKeyRequest{Id: "r1"}))
	if err != nil {
		t.Fatalf("GetApiKey: %v", err)
	}
	if resp.Msg.GetApiKey() != "secret" {
		t.Fatalf("entry = %+v", resp.Msg)
	}

	_, err = server.GetApiKey(context.Background(), connect.NewRequest(&metarrv1.GetApiKeyRequest{Id: "nope"}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", connect.CodeOf(err))
	}
}

func TestUpdateApiKey_AppliesTheMaskAndPinsTheID(t *testing.T) {
	server, backend := newTestApiKeyServer(&appconfig.Config{ApiKeys: &appconfig.APIKeysConfig{
		User: []*appconfig.APIKeyEntry{{Id: "u1", Name: "old", ApiKey: "old-key"}},
	}})

	resp, err := server.UpdateApiKey(correlation.WithID(context.Background(), "c1"),
		connect.NewRequest(&metarrv1.UpdateApiKeyRequest{
			ApiKey:     &metarrv1.APIKeyEntry{Id: "u1", ApiKey: "new-key", Name: "ignored"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"api_key"}},
		}))
	if err != nil {
		t.Fatalf("UpdateApiKey: %v", err)
	}
	if resp.Msg.GetId() != "u1" || resp.Msg.GetApiKey() != "new-key" || resp.Msg.GetName() != "old" {
		t.Fatalf("masked update wrong: %+v", resp.Msg)
	}
	if got := backend.cfg.GetApiKeys().GetUser()[0]; got.GetName() != "old" || got.GetApiKey() != "new-key" {
		t.Fatalf("stored entry = %+v", got)
	}
}

func TestUpdateApiKey_RejectsEmptyMask(t *testing.T) {
	server, _ := newTestApiKeyServer(&appconfig.Config{ApiKeys: &appconfig.APIKeysConfig{
		User: []*appconfig.APIKeyEntry{{Id: "u1", Name: "old"}},
	}})

	_, err := server.UpdateApiKey(context.Background(), connect.NewRequest(&metarrv1.UpdateApiKeyRequest{
		ApiKey: &metarrv1.APIKeyEntry{Id: "u1", Name: "new"},
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestUpdateApiKey_RejectsUnknownPath(t *testing.T) {
	server, _ := newTestApiKeyServer(&appconfig.Config{ApiKeys: &appconfig.APIKeysConfig{
		User: []*appconfig.APIKeyEntry{{Id: "u1", Name: "old"}},
	}})

	_, err := server.UpdateApiKey(context.Background(), connect.NewRequest(&metarrv1.UpdateApiKeyRequest{
		ApiKey:     &metarrv1.APIKeyEntry{Id: "u1"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"nonexistent"}},
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestUpdateApiKey_RejectsMaskNamingID(t *testing.T) {
	server, _ := newTestApiKeyServer(&appconfig.Config{ApiKeys: &appconfig.APIKeysConfig{
		User: []*appconfig.APIKeyEntry{{Id: "u1", Name: "old"}},
	}})

	_, err := server.UpdateApiKey(context.Background(), connect.NewRequest(&metarrv1.UpdateApiKeyRequest{
		ApiKey:     &metarrv1.APIKeyEntry{Id: "u1"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"id"}},
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestListApiKeys_RejectsUnknownOrderByAndUnsupportedFilter(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{ApiKeys: &appconfig.APIKeysConfig{
		User: []*appconfig.APIKeyEntry{{Id: "u1", Name: "one"}},
	}})
	server, _ := newTestApiKeyServer(nil)

	_, err := server.ListApiKeys(context.Background(), connect.NewRequest(&metarrv1.ListApiKeysRequest{
		AccessLevel: metarrv1.AccessLevel_ACCESS_LEVEL_USER,
		OrderBy:     "not_a_field",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("order_by: code = %v, want InvalidArgument", connect.CodeOf(err))
	}

	_, err = server.ListApiKeys(context.Background(), connect.NewRequest(&metarrv1.ListApiKeysRequest{
		AccessLevel: metarrv1.AccessLevel_ACCESS_LEVEL_USER,
		Filter:      "name = 'one'",
	}))
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("filter: code = %v, want Unimplemented", connect.CodeOf(err))
	}
}

func TestUpdateApiKey_UnknownIDIsNotFound(t *testing.T) {
	server, backend := newTestApiKeyServer(&appconfig.Config{ApiKeys: &appconfig.APIKeysConfig{
		Admin: []*appconfig.APIKeyEntry{{Id: "a1", Name: "still-here"}},
	}})

	_, err := server.UpdateApiKey(context.Background(), connect.NewRequest(&metarrv1.UpdateApiKeyRequest{
		ApiKey:     &metarrv1.APIKeyEntry{Id: "ghost", Name: "resurrected?"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", connect.CodeOf(err))
	}
	if len(backend.cfg.GetApiKeys().GetAdmin()) != 1 {
		t.Fatalf("group disturbed by a rejected update: %+v", backend.cfg.GetApiKeys().GetAdmin())
	}
}

func TestDeleteApiKey_RemovesByIDAndIsNotFoundForUnknown(t *testing.T) {
	server, backend := newTestApiKeyServer(&appconfig.Config{ApiKeys: &appconfig.APIKeysConfig{
		Webhook: []*appconfig.APIKeyEntry{{Id: "keep"}, {Id: "drop"}},
	}})

	_, err := server.DeleteApiKey(correlation.WithID(context.Background(), "c1"),
		connect.NewRequest(&metarrv1.DeleteApiKeyRequest{Id: "drop"}))
	if err != nil {
		t.Fatalf("DeleteApiKey: %v", err)
	}
	if kept := backend.cfg.GetApiKeys().GetWebhook(); len(kept) != 1 || kept[0].GetId() != "keep" {
		t.Fatalf("after delete = %+v", kept)
	}

	_, err = server.DeleteApiKey(context.Background(), connect.NewRequest(&metarrv1.DeleteApiKeyRequest{Id: "drop"}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", connect.CodeOf(err))
	}
}
