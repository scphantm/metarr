package services

import (
	"context"
	"log/slog"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/agentregistry"
	"Metarr/internal/server/appconfigstore"
	"Metarr/internal/server/handlers"
	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

func configWithTwoLibraries() *appconfig.Config {
	return &appconfig.Config{
		DirectoryScanner: &appconfig.DirectoryScannerConfig{
			ScanDirectories: []*appconfig.ScanDirectory{
				{ScannerSlug: "movies", ScanType: "movie", Directory: "/media/movies"},
				{ScannerSlug: "tv", ScanType: "tv", Directory: "/media/tv"},
			},
		},
		Agents: []*appconfig.Agent{{
			Slug: "nas-01",
			Mappings: []*appconfig.AgentDirectoryMapping{
				{ScannerSlug: "movies", AgentPath: "/mnt/tank/movies"},
			},
		}},
	}
}

func TestValidateMappingsAcceptsAnUnclaimedLibrary(t *testing.T) {
	entry := &appconfig.Agent{
		Slug: "desktop",
		Mappings: []*appconfig.AgentDirectoryMapping{
			{ScannerSlug: "tv", AgentPath: "/srv/tv"},
		},
	}

	if status, err := validateMappings(configWithTwoLibraries(), entry); err != nil {
		t.Errorf("validateMappings = %d, %v; want accepted", status, err)
	}
}

// Re-saving an agent must not trip the ownership check on its own mappings,
// which is what every edit through the UI does.
func TestValidateMappingsLetsAnAgentKeepItsOwnLibrary(t *testing.T) {
	entry := &appconfig.Agent{
		Slug: "nas-01",
		Mappings: []*appconfig.AgentDirectoryMapping{
			{ScannerSlug: "movies", AgentPath: "/mnt/tank/movies-renamed"},
		},
	}

	if status, err := validateMappings(configWithTwoLibraries(), entry); err != nil {
		t.Errorf("validateMappings = %d, %v; want accepted", status, err)
	}
}

// Two agents scanning one library would each overwrite the other's records
// with its own view of the same files, so the second mapping is a conflict.
func TestValidateMappingsRejectsALibraryClaimedByAnotherAgent(t *testing.T) {
	entry := &appconfig.Agent{
		Slug: "desktop",
		Mappings: []*appconfig.AgentDirectoryMapping{
			{ScannerSlug: "movies", AgentPath: "/srv/movies"},
		},
	}

	status, err := validateMappings(configWithTwoLibraries(), entry)
	if err == nil {
		t.Fatal("validateMappings accepted a library already mapped to another agent")
	}
	if status != http.StatusConflict {
		t.Errorf("status = %d, want %d", status, http.StatusConflict)
	}
}

func TestValidateMappingsRejectsAnUnknownScanDirectory(t *testing.T) {
	entry := &appconfig.Agent{
		Slug: "desktop",
		Mappings: []*appconfig.AgentDirectoryMapping{
			{ScannerSlug: "does-not-exist", AgentPath: "/srv/whatever"},
		},
	}

	status, err := validateMappings(configWithTwoLibraries(), entry)
	if err == nil {
		t.Fatal("validateMappings accepted a mapping to a scan directory that does not exist")
	}
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

// A duplicate would make the projection ambiguous about which path to walk.
func TestValidateMappingsRejectsTheSameLibraryTwice(t *testing.T) {
	entry := &appconfig.Agent{
		Slug: "desktop",
		Mappings: []*appconfig.AgentDirectoryMapping{
			{ScannerSlug: "tv", AgentPath: "/srv/tv"},
			{ScannerSlug: "tv", AgentPath: "/srv/tv-other"},
		},
	}

	status, err := validateMappings(configWithTwoLibraries(), entry)
	if err == nil {
		t.Fatal("validateMappings accepted the same scan directory mapped twice")
	}
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

// An agent with no mappings is a valid, useful state: it is how you register a
// machine before deciding what it may see.
func TestValidateMappingsAcceptsAnAgentWithNoMappings(t *testing.T) {
	entry := &appconfig.Agent{Slug: "brand-new"}

	if status, err := validateMappings(configWithTwoLibraries(), entry); err != nil {
		t.Errorf("validateMappings = %d, %v; want accepted", status, err)
	}
}

func TestAgentForScannerFindsTheOwningAgent(t *testing.T) {
	config := configWithTwoLibraries()

	agent, ok := appconfig.AgentForScanner(config, "movies")
	if !ok || agent.Slug != "nas-01" {
		t.Errorf("AgentForScanner(movies) = %+v, %v; want nas-01", agent, ok)
	}
	if _, ok := appconfig.AgentForScanner(config, "tv"); ok {
		t.Error("AgentForScanner claimed an owner for an unmapped library")
	}
}

// --- Collection matrix + presence (issue #92) -------------------------------

func newTestAgentServer(t *testing.T, seed *appconfig.Config) (*AgentServer, *fakeConfigBackend, redis.UniversalClient) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	backend := &fakeConfigBackend{cfg: seed}
	store := appconfigstore.New(backend, backend, backend)
	// bus is nil: List / Forget never touch it, and no test here publishes a
	// projection.
	registry := agentregistry.New(client, nil, slog.Default())
	server := &AgentServer{Handlers: &handlers.Handlers{
		AppConfigStore: store,
		Agents:         registry,
		Logger:         slog.Default(),
	}}
	return server, backend, client
}

func agentReadServer(t *testing.T) (*AgentServer, redis.UniversalClient) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &AgentServer{Handlers: &handlers.Handlers{
		Agents: agentregistry.New(client, nil, slog.Default()),
		Logger: slog.Default(),
	}}, client
}

func writePresence(t *testing.T, client redis.UniversalClient, slug string) {
	t.Helper()
	payload, err := agentproto.MarshalStored(&agentproto.AgentPresence{
		Identity:  &agentproto.AgentIdentity{Hostname: slug + "-host", Version: "1.0.0"},
		Telemetry: &agentproto.AgentTelemetry{CpuPercent: 12.5},
	})
	if err != nil {
		t.Fatalf("marshal presence: %v", err)
	}
	if err := client.Set(context.Background(), agentproto.PresenceKey(slug), payload, 0).Err(); err != nil {
		t.Fatalf("write presence: %v", err)
	}
}

func agentCreateReq(id string, agent *metarrv1.Agent) *connect.Request[metarrv1.CreateAgentRequest] {
	return connect.NewRequest(&metarrv1.CreateAgentRequest{AgentId: id, Agent: agent})
}

func TestAgentCreate_AppendsAndIgnoresPresenceInRequest(t *testing.T) {
	server, backend, _ := newTestAgentServer(t, configWithTwoLibraries())

	ctx := correlation.WithID(context.Background(), "corr-1")
	resp, err := server.CreateAgent(ctx, agentCreateReq("desktop", &metarrv1.Agent{
		DisplayName: "Desktop",
		Mappings:    []*metarrv1.AgentDirectoryMapping{{ScannerSlug: "tv", AgentPath: "/srv/tv"}},
		// Presence fields in the request must be ignored.
		Online:     true,
		Configured: true,
		Identity:   &agentproto.AgentIdentity{Hostname: "spoofed"},
	}))
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if resp.Msg.GetSlug() != "desktop" || !resp.Msg.GetConfigured() {
		t.Errorf("response = %+v, want slug desktop / configured true", resp.Msg)
	}
	if resp.Msg.GetOnline() || resp.Msg.GetIdentity() != nil {
		t.Errorf("response carried presence the request tried to set: %+v", resp.Msg)
	}
	stored := backend.cfg.GetAgents()
	if len(stored) != 2 {
		t.Fatalf("persisted agents = %d, want 2", len(stored))
	}
	created := stored[len(stored)-1]
	if created.GetOnline() || created.GetConfigured() || created.GetIdentity() != nil || created.GetTelemetry() != nil {
		t.Errorf("presence reached the persisted document: %+v", created)
	}
	if len(backend.fired) != 0 {
		t.Fatalf("a synchronous write fired %d events, want 0", len(backend.fired))
	}
}

func TestAgentCreate_ExistingSlugIsAlreadyExists(t *testing.T) {
	server, _, _ := newTestAgentServer(t, configWithTwoLibraries())

	_, err := server.CreateAgent(context.Background(), agentCreateReq("nas-01", &metarrv1.Agent{}))
	if got := connect.CodeOf(err); got != connect.CodeAlreadyExists {
		t.Fatalf("code = %v, want AlreadyExists", got)
	}
}

func TestAgentCreate_SlugBodyMismatchIsInvalidArgument(t *testing.T) {
	server, _, _ := newTestAgentServer(t, configWithTwoLibraries())

	_, err := server.CreateAgent(context.Background(), agentCreateReq("desktop", &metarrv1.Agent{Slug: "other"}))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", got)
	}
}

// validateMappings is ported verbatim (issue #92) and answers a
// library-already-claimed conflict with HTTP 409, which the shared
// connectError mapping turns into AlreadyExists.
func TestAgentCreate_RejectsAConflictingMapping(t *testing.T) {
	server, _, _ := newTestAgentServer(t, configWithTwoLibraries())

	_, err := server.CreateAgent(context.Background(), agentCreateReq("desktop", &metarrv1.Agent{
		Mappings: []*metarrv1.AgentDirectoryMapping{{ScannerSlug: "movies", AgentPath: "/srv/movies"}},
	}))
	if got := connect.CodeOf(err); got != connect.CodeAlreadyExists {
		t.Fatalf("code = %v, want AlreadyExists (library already mapped to another agent)", got)
	}
}

func TestAgentGet_MergesPresenceOntoTheConfiguredAgent(t *testing.T) {
	server, client := agentReadServer(t)
	withLiveConfig(t, configWithTwoLibraries())
	writePresence(t, client, "nas-01")

	resp, err := server.GetAgent(context.Background(), connect.NewRequest(&metarrv1.GetAgentRequest{Slug: "nas-01"}))
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if !resp.Msg.GetConfigured() || !resp.Msg.GetOnline() {
		t.Errorf("expected configured + online, got %+v", resp.Msg)
	}
	if resp.Msg.GetIdentity().GetHostname() != "nas-01-host" {
		t.Errorf("identity not merged: %+v", resp.Msg.GetIdentity())
	}

	_, err = server.GetAgent(context.Background(), connect.NewRequest(&metarrv1.GetAgentRequest{Slug: "ghost"}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestAgentGet_PresenceOnlyAgentIsNotConfigured(t *testing.T) {
	server, client := agentReadServer(t)
	withLiveConfig(t, configWithTwoLibraries())
	writePresence(t, client, "stranger")

	resp, err := server.GetAgent(context.Background(), connect.NewRequest(&metarrv1.GetAgentRequest{Slug: "stranger"}))
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if resp.Msg.GetConfigured() {
		t.Errorf("a presence-only agent reported configured: %+v", resp.Msg)
	}
	if !resp.Msg.GetOnline() {
		t.Error("a presence-only agent should be online")
	}
}

func TestAgentList_PaginatesOrdersAndRejectsFilter(t *testing.T) {
	server, _ := agentReadServer(t)
	withLiveConfig(t, &appconfig.Config{
		DirectoryScanner: &appconfig.DirectoryScannerConfig{},
		Agents: []*appconfig.Agent{
			{Slug: "c"}, {Slug: "a"}, {Slug: "b"},
		},
	})

	first, err := server.ListAgents(context.Background(), connect.NewRequest(&metarrv1.ListAgentsRequest{
		PageSize: 2, OrderBy: "slug",
	}))
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(first.Msg.GetAgents()) != 2 || first.Msg.GetNextPageToken() == "" {
		t.Fatalf("page 1 = %d agents, token %q", len(first.Msg.GetAgents()), first.Msg.GetNextPageToken())
	}
	if first.Msg.GetAgents()[0].GetSlug() != "a" {
		t.Errorf("order = %q, want a first", first.Msg.GetAgents()[0].GetSlug())
	}

	_, err = server.ListAgents(context.Background(), connect.NewRequest(&metarrv1.ListAgentsRequest{
		Filter: `slug = "a"`,
	}))
	if got := connect.CodeOf(err); got != connect.CodeUnimplemented {
		t.Fatalf("code = %v, want Unimplemented", got)
	}
}

func TestAgentUpdate_PartialMaskLeavesSiblingsAndNeverStoresPresence(t *testing.T) {
	server, backend, _ := newTestAgentServer(t, configWithTwoLibraries())

	_, err := server.UpdateAgent(context.Background(), connect.NewRequest(&metarrv1.UpdateAgentRequest{
		Agent: &metarrv1.Agent{
			Slug:        "nas-01",
			DisplayName: "Renamed",
			LogLevel:    "debug",
			Online:      true,
			Identity:    &agentproto.AgentIdentity{Hostname: "spoofed"},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name"}},
	}))
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	got := backend.cfg.GetAgents()[0]
	if got.GetDisplayName() != "Renamed" {
		t.Errorf("display_name = %q, want Renamed", got.GetDisplayName())
	}
	if got.GetLogLevel() != "" {
		t.Errorf("log_level = %q, want unchanged — an unmasked field moved", got.GetLogLevel())
	}
	if len(got.GetMappings()) != 1 || got.GetMappings()[0].GetScannerSlug() != "movies" {
		t.Errorf("mappings moved on an unmasked update: %+v", got.GetMappings())
	}
	if got.GetOnline() || got.GetIdentity() != nil {
		t.Errorf("presence reached the persisted document: %+v", got)
	}
}

func TestAgentUpdate_MaskErrorsAndAllowMissing(t *testing.T) {
	seed := configWithTwoLibraries

	t.Run("empty mask", func(t *testing.T) {
		server, _, _ := newTestAgentServer(t, seed())
		_, err := server.UpdateAgent(context.Background(), connect.NewRequest(&metarrv1.UpdateAgentRequest{
			Agent: &metarrv1.Agent{Slug: "nas-01", DisplayName: "x"},
		}))
		if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
			t.Fatalf("code = %v, want InvalidArgument", got)
		}
	})
	t.Run("output-only field in mask", func(t *testing.T) {
		server, _, _ := newTestAgentServer(t, seed())
		_, err := server.UpdateAgent(context.Background(), connect.NewRequest(&metarrv1.UpdateAgentRequest{
			Agent:      &metarrv1.Agent{Slug: "nas-01"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"online"}},
		}))
		if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
			t.Fatalf("code = %v, want InvalidArgument", got)
		}
	})
	t.Run("unknown slug without allow_missing", func(t *testing.T) {
		server, _, _ := newTestAgentServer(t, seed())
		_, err := server.UpdateAgent(context.Background(), connect.NewRequest(&metarrv1.UpdateAgentRequest{
			Agent:      &metarrv1.Agent{Slug: "ghost", DisplayName: "x"},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name"}},
		}))
		if got := connect.CodeOf(err); got != connect.CodeNotFound {
			t.Fatalf("code = %v, want NotFound", got)
		}
	})
	t.Run("allow_missing creates", func(t *testing.T) {
		server, backend, _ := newTestAgentServer(t, seed())
		resp, err := server.UpdateAgent(context.Background(), connect.NewRequest(&metarrv1.UpdateAgentRequest{
			Agent:        &metarrv1.Agent{Slug: "fresh", DisplayName: "Fresh"},
			UpdateMask:   &fieldmaskpb.FieldMask{Paths: []string{"display_name"}},
			AllowMissing: true,
		}))
		if err != nil {
			t.Fatalf("UpdateAgent allow_missing: %v", err)
		}
		if resp.Msg.GetSlug() != "fresh" || !resp.Msg.GetConfigured() {
			t.Errorf("response = %+v", resp.Msg)
		}
		if len(backend.cfg.GetAgents()) != 2 {
			t.Fatalf("persisted agents = %d, want 2", len(backend.cfg.GetAgents()))
		}
	})
}

func TestAgentDelete_RemovesAndForgetsProjection(t *testing.T) {
	server, backend, client := newTestAgentServer(t, configWithTwoLibraries())
	if err := client.Set(context.Background(), agentproto.ConfigKey("nas-01"), "x", 0).Err(); err != nil {
		t.Fatalf("seed projection: %v", err)
	}

	if _, err := server.DeleteAgent(context.Background(), connect.NewRequest(&metarrv1.DeleteAgentRequest{Slug: "nas-01"})); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	if len(backend.cfg.GetAgents()) != 0 {
		t.Fatalf("agent was not removed: %+v", backend.cfg.GetAgents())
	}
	if err := client.Get(context.Background(), agentproto.ConfigKey("nas-01")).Err(); err != redis.Nil {
		t.Errorf("projection key was not forgotten: %v", err)
	}

	_, err := server.DeleteAgent(context.Background(), connect.NewRequest(&metarrv1.DeleteAgentRequest{Slug: "nas-01"}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestAgentSetLogLevel_UpdatesAndCreatesBareEntry(t *testing.T) {
	server, backend, _ := newTestAgentServer(t, configWithTwoLibraries())

	resp, err := server.SetLogLevel(context.Background(), connect.NewRequest(&metarrv1.SetLogLevelRequest{
		Slug: "nas-01", LogLevel: "debug",
	}))
	if err != nil {
		t.Fatalf("SetLogLevel: %v", err)
	}
	if resp.Msg.GetLogLevel() != "debug" || !resp.Msg.GetConfigured() {
		t.Errorf("response = %+v", resp.Msg)
	}
	if backend.cfg.GetAgents()[0].GetLogLevel() != "debug" {
		t.Errorf("persisted log_level = %q, want debug", backend.cfg.GetAgents()[0].GetLogLevel())
	}

	if _, err := server.SetLogLevel(context.Background(), connect.NewRequest(&metarrv1.SetLogLevelRequest{
		Slug: "brand-new", LogLevel: "debug",
	})); err != nil {
		t.Fatalf("SetLogLevel for an unconfigured agent: %v", err)
	}
	if got := backend.cfg.GetAgents(); len(got) != 2 || got[1].GetSlug() != "brand-new" {
		t.Fatalf("a bare entry was not created: %+v", got)
	}

	_, err = server.SetLogLevel(context.Background(), connect.NewRequest(&metarrv1.SetLogLevelRequest{
		Slug: "nas-01", LogLevel: "loud",
	}))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument for a bad level", got)
	}
}
