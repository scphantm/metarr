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

func newTestEventBusServer(seed *appconfig.Config) (*EventBusServer, *fakeConfigBackend, *fakeOperationStore) {
	backend := &fakeConfigBackend{cfg: seed}
	ops := newFakeOperationStore()
	store := appconfigstore.New(backend, backend, backend)
	return &EventBusServer{Handlers: &handlers.Handlers{
		AppConfigStore: store,
		Operations:     ops,
		Logger:         slog.Default(),
	}}, backend, ops
}

func validEventBusConfig() *metarrv1.EventBusConfig {
	return &metarrv1.EventBusConfig{
		MaxLen: 10000, RetentionHours: 48,
		RetryAttempts: 4, RetryBackoffBaseMs: 500, RetryBackoffMaxMs: 30000,
	}
}

// fullEventBusMask names every EventBusConfig field — a mask-driven update
// with it is a whole-section replace, the shape the pre-AIP UpdateConfig had.
func fullEventBusMask() *fieldmaskpb.FieldMask {
	return &fieldmaskpb.FieldMask{Paths: []string{
		"max_len", "retention_hours", "retry_attempts",
		"retry_backoff_base_ms", "retry_backoff_max_ms",
	}}
}

func TestEventBusUpdateEventBusConfig_WritesThroughAScopedMutation(t *testing.T) {
	// A seed carrying an admin credential: a scoped mutation must touch only
	// event_bus and leave everything else byte-identical.
	seed := &appconfig.Config{
		Admin:    &appconfig.AdminUser{Username: "admin", PasswordHash: "keep-me"},
		EventBus: &appconfig.EventBusConfig{RetentionHours: 48, MaxLen: 1, RetryBackoffBaseMs: 1, RetryBackoffMaxMs: 1},
	}
	server, backend, ops := newTestEventBusServer(seed)

	ctx := correlation.WithID(context.Background(), "corr-eb-1")
	next := validEventBusConfig()
	next.RetentionHours = 72
	resp, err := server.UpdateEventBusConfig(ctx, connect.NewRequest(&metarrv1.UpdateEventBusConfigRequest{
		Config:     next,
		UpdateMask: fullEventBusMask(),
	}))
	if err != nil {
		t.Fatalf("UpdateEventBusConfig: %v", err)
	}

	if got := backend.cfg.GetEventBus().GetRetentionHours(); got != 72 {
		t.Errorf("retention_hours persisted as %d, want 72", got)
	}
	if backend.cfg.GetAdmin().GetPasswordHash() != "keep-me" {
		t.Errorf("a scoped event_bus write disturbed the admin credential: %+v", backend.cfg.GetAdmin())
	}
	if backend.cfg.GetEventBus().GetEtag() != "" {
		t.Errorf("a derived etag reached the stored document: %q", backend.cfg.GetEventBus().GetEtag())
	}
	if len(backend.fired) != 1 {
		t.Fatalf("expected exactly one system_config_update event, got %d", len(backend.fired))
	}
	if backend.fired[0].CorrelationId != "corr-eb-1" {
		t.Errorf("fired event correlation id = %q, want %q", backend.fired[0].CorrelationId, "corr-eb-1")
	}

	// The write returns an unfinished operation named for the correlation id,
	// and it is recorded in the operation store for the listener to finish.
	if resp.Msg.GetName() != "operations/corr-eb-1" || resp.Msg.GetDone() {
		t.Errorf("Operation = %+v, want name=operations/corr-eb-1 done=false", resp.Msg)
	}
	if _, ok := ops.ops["operations/corr-eb-1"]; !ok {
		t.Errorf("the operation was not recorded: %+v", ops.ops)
	}
}

// A partial mask changes exactly the fields it names and leaves every sibling
// on the stored section untouched. EventBusConfig is a flat block of scalars,
// so a single-path mask is the "nested field" case here; dotted descent through
// a scalar is exercised by the unknown-path test below.
func TestEventBusUpdateEventBusConfig_AppliesMaskPartially(t *testing.T) {
	seed := &appconfig.Config{EventBus: validEventBusConfig()}
	server, backend, _ := newTestEventBusServer(seed)

	// Carry deliberately wrong values for the unmasked fields: only
	// retention_hours is named, so only it may move.
	_, err := server.UpdateEventBusConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateEventBusConfigRequest{
		Config: &metarrv1.EventBusConfig{
			MaxLen: 999999, RetentionHours: 96, RetryAttempts: 99,
			RetryBackoffBaseMs: 1, RetryBackoffMaxMs: 2,
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"retention_hours"}},
	}))
	if err != nil {
		t.Fatalf("UpdateEventBusConfig: %v", err)
	}

	got := backend.cfg.GetEventBus()
	if got.GetRetentionHours() != 96 {
		t.Errorf("retention_hours = %d, want 96", got.GetRetentionHours())
	}
	if got.GetMaxLen() != 10000 {
		t.Errorf("max_len = %d, want the seeded 10000 — an unmasked field moved", got.GetMaxLen())
	}
	if got.GetRetryAttempts() != 4 || got.GetRetryBackoffBaseMs() != 500 || got.GetRetryBackoffMaxMs() != 30000 {
		t.Errorf("an unmasked retry field moved: %+v", got)
	}
}

func TestEventBusUpdateEventBusConfig_RejectsEmptyMask(t *testing.T) {
	server, backend, _ := newTestEventBusServer(&appconfig.Config{EventBus: validEventBusConfig()})

	_, err := server.UpdateEventBusConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateEventBusConfigRequest{
		Config: validEventBusConfig(),
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

func TestEventBusUpdateEventBusConfig_RejectsUnknownPath(t *testing.T) {
	server, backend, _ := newTestEventBusServer(&appconfig.Config{EventBus: validEventBusConfig()})

	cases := map[string]string{
		"no such field":          "max_length",
		"descend through scalar": "retention_hours.value",
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := server.UpdateEventBusConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateEventBusConfigRequest{
				Config:     validEventBusConfig(),
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

func TestEventBusUpdateEventBusConfig_RejectsAStaleETag(t *testing.T) {
	seed := validEventBusConfig()
	server, backend, _ := newTestEventBusServer(&appconfig.Config{EventBus: seed})

	// The token a client read before anyone else wrote.
	staleETag := sectionETag(seed)

	// First write carries the still-current token and lands.
	_, err := server.UpdateEventBusConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateEventBusConfigRequest{
		Config:     &metarrv1.EventBusConfig{RetentionHours: 72},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"retention_hours"}},
		Etag:       staleETag,
	}))
	if err != nil {
		t.Fatalf("first update: %v", err)
	}

	// The section has moved; the same token is now stale.
	_, err = server.UpdateEventBusConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateEventBusConfigRequest{
		Config:     &metarrv1.EventBusConfig{RetentionHours: 96},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"retention_hours"}},
		Etag:       staleETag,
	}))
	if connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("code = %v, want Aborted", connect.CodeOf(err))
	}
	if got := backend.cfg.GetEventBus().GetRetentionHours(); got != 72 {
		t.Errorf("the aborted write still moved retention_hours to %d", got)
	}
	if len(backend.fired) != 1 {
		t.Errorf("the aborted write fired an event: %d total", len(backend.fired))
	}
}

func TestEventBusUpdateEventBusConfig_RejectsAnInvalidSection(t *testing.T) {
	server, backend, _ := newTestEventBusServer(&appconfig.Config{EventBus: validEventBusConfig()})

	cases := map[string]func(*metarrv1.EventBusConfig){
		"zero max_len":           func(c *metarrv1.EventBusConfig) { c.MaxLen = 0 },
		"negative max_len":       func(c *metarrv1.EventBusConfig) { c.MaxLen = -1 },
		"zero retention":         func(c *metarrv1.EventBusConfig) { c.RetentionHours = 0 },
		"max backoff below base": func(c *metarrv1.EventBusConfig) { c.RetryBackoffMaxMs = 1; c.RetryBackoffBaseMs = 500 },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := validEventBusConfig()
			mutate(cfg)
			_, err := server.UpdateEventBusConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateEventBusConfigRequest{
				Config:     cfg,
				UpdateMask: fullEventBusMask(),
			}))
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

func TestEventBusGetEventBusConfig_ReadsLiveConfigWithAnETag(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		EventBus: &appconfig.EventBusConfig{MaxLen: 12345, RetentionHours: 96},
	})
	server := &EventBusServer{Handlers: &handlers.Handlers{}}

	resp, err := server.GetEventBusConfig(context.Background(), connect.NewRequest(&metarrv1.GetEventBusConfigRequest{}))
	if err != nil {
		t.Fatalf("GetEventBusConfig: %v", err)
	}
	if got := resp.Msg.GetConfig().GetMaxLen(); got != 12345 {
		t.Errorf("max_len = %d, want 12345", got)
	}
	if got := resp.Msg.GetConfig().GetRetentionHours(); got != 96 {
		t.Errorf("retention_hours = %d, want 96", got)
	}
	if resp.Msg.GetConfig().GetEtag() == "" {
		t.Error("the read carried no etag")
	}
	// The live config singleton must never hold a derived etag.
	if appconfig.Get().EventBus.GetEtag() != "" {
		t.Errorf("GetEventBusConfig stamped an etag onto live config: %q", appconfig.Get().EventBus.GetEtag())
	}
}
