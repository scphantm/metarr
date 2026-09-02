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

func newTestLoggingServer(seed *appconfig.Config) (*LoggingServer, *fakeConfigBackend) {
	backend := &fakeConfigBackend{cfg: seed}
	store := appconfigstore.New(backend, backend, backend)
	return &LoggingServer{Handlers: &handlers.Handlers{
		AppConfigStore: store,
		Logger:         slog.Default(),
	}}, backend
}

func seededLoggingConfig() *metarrv1.LoggingConfig {
	return &metarrv1.LoggingConfig{
		ServerLevel: appconfig.LogLevelInfo,
		Sink:        "fluent-bit",
		Endpoint:    "http://openobserve.example/logs",
		Stream:      "metarr",
	}
}

func TestLoggingUpdateLoggingConfig_WritesThroughAScopedMutation(t *testing.T) {
	seed := &appconfig.Config{
		Admin:   &appconfig.AdminUser{Username: "admin", PasswordHash: "keep-me"},
		Logging: seededLoggingConfig(),
	}
	server, backend := newTestLoggingServer(seed)

	ctx := correlation.WithID(context.Background(), "corr-log-1")
	resp, err := server.UpdateLoggingConfig(ctx, connect.NewRequest(&metarrv1.UpdateLoggingConfigRequest{
		Config:     &metarrv1.LoggingConfig{ServerLevel: appconfig.LogLevelDebug},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"server_level"}},
	}))
	if err != nil {
		t.Fatalf("UpdateLoggingConfig: %v", err)
	}

	if got := backend.cfg.GetLogging().GetServerLevel(); got != appconfig.LogLevelDebug {
		t.Errorf("server_level persisted as %q, want %q", got, appconfig.LogLevelDebug)
	}
	if backend.cfg.GetAdmin().GetPasswordHash() != "keep-me" {
		t.Errorf("a scoped logging write disturbed the admin credential: %+v", backend.cfg.GetAdmin())
	}
	if len(backend.fired) != 0 {
		t.Fatalf("a synchronous write fired %d system_config_update events, want 0", len(backend.fired))
	}
	if resp.Msg.GetServerLevel() != appconfig.LogLevelDebug {
		t.Errorf("UpdateLoggingConfig returned server_level %q, want %q", resp.Msg.GetServerLevel(), appconfig.LogLevelDebug)
	}
}

// A partial mask changes exactly the field it names; the informational
// pipeline fields and the server level around it stay as stored even when the
// request body carries other values.
func TestLoggingUpdateLoggingConfig_AppliesMaskPartially(t *testing.T) {
	server, backend := newTestLoggingServer(&appconfig.Config{Logging: seededLoggingConfig()})

	_, err := server.UpdateLoggingConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateLoggingConfigRequest{
		Config: &metarrv1.LoggingConfig{
			ServerLevel: "garbage-that-must-be-ignored",
			Sink:        "splunk",
			Endpoint:    "http://elsewhere.example",
			Stream:      "other",
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"sink"}},
	}))
	if err != nil {
		t.Fatalf("UpdateLoggingConfig: %v", err)
	}

	got := backend.cfg.GetLogging()
	if got.GetSink() != "splunk" {
		t.Errorf("sink = %q, want %q", got.GetSink(), "splunk")
	}
	if got.GetServerLevel() != appconfig.LogLevelInfo {
		t.Errorf("server_level = %q, want the seeded %q — an unmasked field moved", got.GetServerLevel(), appconfig.LogLevelInfo)
	}
	if got.GetEndpoint() != "http://openobserve.example/logs" || got.GetStream() != "metarr" {
		t.Errorf("an unmasked pipeline field moved: %+v", got)
	}
}

func TestLoggingUpdateLoggingConfig_RejectsEmptyMask(t *testing.T) {
	server, backend := newTestLoggingServer(&appconfig.Config{Logging: seededLoggingConfig()})

	_, err := server.UpdateLoggingConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateLoggingConfigRequest{
		Config: seededLoggingConfig(),
	}))
	if err == nil {
		t.Fatal("expected an error for an absent update_mask")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
	if len(backend.fired) != 0 {
		t.Errorf("a rejected update fired %d events", len(backend.fired))
	}
}

func TestLoggingUpdateLoggingConfig_RejectsAMissingConfig(t *testing.T) {
	server, backend := newTestLoggingServer(&appconfig.Config{Logging: seededLoggingConfig()})

	_, err := server.UpdateLoggingConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateLoggingConfigRequest{
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"server_level"}},
	}))
	if err == nil {
		t.Fatal("expected an error for a nil config")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
	if len(backend.fired) != 0 {
		t.Errorf("a rejected update fired %d events", len(backend.fired))
	}
}

func TestLoggingUpdateLoggingConfig_RejectsUnknownPath(t *testing.T) {
	server, backend := newTestLoggingServer(&appconfig.Config{Logging: seededLoggingConfig()})

	cases := map[string]string{
		"no such field":          "level",
		"descend through scalar": "server_level.name",
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := server.UpdateLoggingConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateLoggingConfigRequest{
				Config:     seededLoggingConfig(),
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{path}},
			}))
			if err == nil {
				t.Fatalf("expected an error for mask path %q", path)
			}
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
			}
		})
	}
	if len(backend.fired) != 0 {
		t.Errorf("a rejected update fired %d events", len(backend.fired))
	}
}

func TestLoggingUpdateLoggingConfig_RejectsAnInvalidLevel(t *testing.T) {
	server, backend := newTestLoggingServer(&appconfig.Config{Logging: seededLoggingConfig()})

	_, err := server.UpdateLoggingConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateLoggingConfigRequest{
		Config:     &metarrv1.LoggingConfig{ServerLevel: "trace"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"server_level"}},
	}))
	if err == nil {
		t.Fatal("expected an error for an out-of-range level")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
	if len(backend.fired) != 0 {
		t.Errorf("a rejected update fired %d events", len(backend.fired))
	}
}

// The synchronous write propagates in-process before returning, so the next
// GetLoggingConfig — reading live config — already sees it with no
// system_config_update round trip.
func TestLoggingUpdateLoggingConfig_VisibleOnNextGet(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{Logging: seededLoggingConfig()})
	server, _ := newTestLoggingServer(&appconfig.Config{Logging: seededLoggingConfig()})
	server.AppConfigStore.SetPropagator(liveConfigPropagator{})

	if _, err := server.UpdateLoggingConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateLoggingConfigRequest{
		Config:     &metarrv1.LoggingConfig{ServerLevel: appconfig.LogLevelDebug},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"server_level"}},
	})); err != nil {
		t.Fatalf("UpdateLoggingConfig: %v", err)
	}

	got, err := server.GetLoggingConfig(context.Background(), connect.NewRequest(&metarrv1.GetLoggingConfigRequest{}))
	if err != nil {
		t.Fatalf("GetLoggingConfig: %v", err)
	}
	if got.Msg.GetConfig().GetServerLevel() != appconfig.LogLevelDebug {
		t.Errorf("Get after Update returned server_level %q, want %q", got.Msg.GetConfig().GetServerLevel(), appconfig.LogLevelDebug)
	}
}

func TestLoggingGetLoggingConfig_ReadsLiveConfig(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		Logging: &appconfig.LoggingConfig{ServerLevel: appconfig.LogLevelDebug, Sink: "fluent-bit"},
	})
	server := &LoggingServer{Handlers: &handlers.Handlers{}}

	resp, err := server.GetLoggingConfig(context.Background(), connect.NewRequest(&metarrv1.GetLoggingConfigRequest{}))
	if err != nil {
		t.Fatalf("GetLoggingConfig: %v", err)
	}
	if got := resp.Msg.GetConfig().GetServerLevel(); got != appconfig.LogLevelDebug {
		t.Errorf("server_level = %q, want %q", got, appconfig.LogLevelDebug)
	}
	if got := resp.Msg.GetConfig().GetSink(); got != "fluent-bit" {
		t.Errorf("sink = %q, want %q", got, "fluent-bit")
	}
	// The response is a clone: mutating it must not reach live config.
	resp.Msg.GetConfig().Sink = "changed"
	if appconfig.Get().Logging.GetSink() != "fluent-bit" {
		t.Error("GetLoggingConfig handed out the live-config pointer")
	}
}
