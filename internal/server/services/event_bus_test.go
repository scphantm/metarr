package services

import (
	"context"
	"log/slog"
	"testing"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/appconfigstore"
	"Metarr/internal/server/handlers"
	"Metarr/internal/shared/appconfig"
)

func newTestEventBusServer(seed *appconfig.Config) (*EventBusServer, *fakeConfigBackend) {
	backend := &fakeConfigBackend{cfg: seed}
	store := appconfigstore.New(backend, backend, backend)
	return &EventBusServer{Handlers: &handlers.Handlers{
		AppConfigStore: store,
		Logger:         slog.Default(),
	}}, backend
}

func validEventBusConfig() *metarrv1.EventBusConfig {
	return &metarrv1.EventBusConfig{
		MaxLenHigh: 100000, MaxLenDefault: 10000, RetentionHours: 48,
		RetryAttempts: 4, RetryBackoffBaseMs: 500, RetryBackoffMaxMs: 30000,
	}
}

func TestEventBusUpdateConfig_WritesThroughAScopedMutation(t *testing.T) {
	// A seed carrying an admin credential: a scoped mutation must touch only
	// event_bus and leave everything else byte-identical.
	seed := &appconfig.Config{
		Admin:    &appconfig.AdminUser{Username: "admin", PasswordHash: "keep-me"},
		EventBus: &appconfig.EventBusConfig{RetentionHours: 48, MaxLenHigh: 1, MaxLenDefault: 1, RetryBackoffBaseMs: 1, RetryBackoffMaxMs: 1},
	}
	server, backend := newTestEventBusServer(seed)

	next := validEventBusConfig()
	next.RetentionHours = 72
	_, err := server.UpdateConfig(context.Background(), connect.NewRequest(&metarrv1.EventBusServiceUpdateConfigRequest{Config: next}))
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	if got := backend.cfg.GetEventBus().GetRetentionHours(); got != 72 {
		t.Errorf("retention_hours persisted as %d, want 72", got)
	}
	if backend.cfg.GetAdmin().GetPasswordHash() != "keep-me" {
		t.Errorf("a scoped event_bus write disturbed the admin credential: %+v", backend.cfg.GetAdmin())
	}
	if len(backend.fired) != 1 {
		t.Errorf("expected exactly one system_config_update event, got %d", len(backend.fired))
	}
}

func TestEventBusUpdateConfig_RejectsAnInvalidSection(t *testing.T) {
	server, backend := newTestEventBusServer(&appconfig.Config{})

	cases := map[string]func(*metarrv1.EventBusConfig){
		"zero max_len_default":       func(c *metarrv1.EventBusConfig) { c.MaxLenDefault = 0 },
		"high cap below default cap": func(c *metarrv1.EventBusConfig) { c.MaxLenHigh = 5; c.MaxLenDefault = 10 },
		"zero retention":             func(c *metarrv1.EventBusConfig) { c.RetentionHours = 0 },
		"max backoff below base":     func(c *metarrv1.EventBusConfig) { c.RetryBackoffMaxMs = 1; c.RetryBackoffBaseMs = 500 },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := validEventBusConfig()
			mutate(cfg)
			_, err := server.UpdateConfig(context.Background(), connect.NewRequest(&metarrv1.EventBusServiceUpdateConfigRequest{Config: cfg}))
			if err == nil {
				t.Fatal("expected a validation error")
			}
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
			}
		})
	}

	if len(backend.fired) != 0 {
		t.Errorf("a rejected update still fired %d events", len(backend.fired))
	}
}

func TestEventBusGetConfig_ReadsLiveConfig(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		EventBus: &appconfig.EventBusConfig{MaxLenHigh: 12345, RetentionHours: 96},
	})
	server := &EventBusServer{Handlers: &handlers.Handlers{}}

	resp, err := server.GetConfig(context.Background(), connect.NewRequest(&metarrv1.EventBusServiceGetConfigRequest{}))
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got := resp.Msg.GetConfig().GetMaxLenHigh(); got != 12345 {
		t.Errorf("max_len_high = %d, want 12345", got)
	}
	if got := resp.Msg.GetConfig().GetRetentionHours(); got != 96 {
		t.Errorf("retention_hours = %d, want 96", got)
	}
}
