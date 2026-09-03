package services

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/handlers"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/eventbus"
)

func newTestTaskServer(t *testing.T) (*TaskServer, redis.UniversalClient) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	bus, err := eventbus.New(eventbus.Config{
		Redis:   client,
		Source:  eventbus.SourceServer,
		Streams: eventbus.RedisStreamTransport(client, eventbus.NewSlogAdapter(slog.Default())),
		Policy:  eventbus.DefaultBusPolicy,
		Logger:  slog.Default(),
	})
	if err != nil {
		t.Fatalf("eventbus.New: %v", err)
	}

	// The bus builds its stream publisher inside Run, so Publish is only live
	// after Ready. This server publishes but consumes nothing.
	runCtx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() { _ = bus.Run(runCtx); close(runDone) }()
	select {
	case <-bus.Ready():
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("bus did not become ready")
	}
	t.Cleanup(func() { cancel(); <-runDone; _ = bus.Close() })

	return &TaskServer{Handlers: &handlers.Handlers{Bus: bus, Logger: slog.Default()}}, client
}

func configWithMappedAgent() *appconfig.Config {
	return &appconfig.Config{
		DirectoryScanner: &appconfig.DirectoryScannerConfig{
			ScanDirectories: []*appconfig.ScanDirectory{
				{ScannerSlug: "movies", ScanType: "movie", Directory: "/media/movies"},
			},
		},
		Agents: []*appconfig.Agent{{
			Slug:     "nas-01",
			Mappings: []*appconfig.AgentDirectoryMapping{{ScannerSlug: "movies", AgentPath: "/mnt/movies"}},
		}},
	}
}

// The point-in-time online check and its 422 are gone: a scan aimed at a
// mapped-but-absent agent is accepted and lands on the durable command
// stream, where the agent consumes it on return.
func TestRunDirectoryScan_DispatchesToAnAbsentAgentWithoutA422(t *testing.T) {
	server, client := newTestTaskServer(t)
	withLiveConfig(t, configWithMappedAgent())

	// In the running server the request middleware has already put a
	// correlation id on the context by the time the handler sees it
	// (internal/server/httpserver/middleware.go); seed one here so the test
	// exercises the same path. The handler echoes it back as scan_id.
	ctx := correlation.WithID(context.Background(), "corr-scan-1")
	resp, err := server.RunDirectoryScan(ctx, connect.NewRequest(&metarrv1.TaskServiceRunDirectoryScanRequest{
		ScannerSlug: "movies",
	}))
	if err != nil {
		t.Fatalf("RunDirectoryScan returned an error for an absent agent: %v", err)
	}
	if resp.Msg.GetScanId() != "corr-scan-1" {
		t.Errorf("scan_id = %q, want the context correlation id %q", resp.Msg.GetScanId(), "corr-scan-1")
	}

	entries, err := client.XRange(context.Background(), eventbus.AgentCommandStream("nas-01"), "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange on the agent command stream: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("agent command stream has %d entries, want 1 queued command", len(entries))
	}
	raw, ok := entries[0].Values["payload"].(string)
	if !ok {
		t.Fatalf("queued command entry has no payload envelope: %v", entries[0].Values)
	}
	var event eventbus.Event
	if err := eventbus.UnmarshalEvent([]byte(raw), &event); err != nil {
		t.Fatalf("payload is not an event envelope: %v", err)
	}
	// It is the agent.scan command, and it carries the same scan id the RPC
	// handed back — a caller that follows scan_id is watching the command that
	// was actually published.
	if event.GetName() != eventbus.AgentScanCommandEventName {
		t.Errorf("queued command event = %q, want %q", event.GetName(), eventbus.AgentScanCommandEventName)
	}
	if event.GetCorrelationId() != resp.Msg.GetScanId() {
		t.Errorf("queued command correlation id = %q, want scan_id %q", event.GetCorrelationId(), resp.Msg.GetScanId())
	}
}

func TestRunDirectoryScan_StillRejectsAnUnmappedScanDirectory(t *testing.T) {
	server, _ := newTestTaskServer(t)
	withLiveConfig(t, &appconfig.Config{
		DirectoryScanner: &appconfig.DirectoryScannerConfig{
			ScanDirectories: []*appconfig.ScanDirectory{
				{ScannerSlug: "movies", ScanType: "movie", Directory: "/media/movies"},
			},
		},
	})

	_, err := server.RunDirectoryScan(context.Background(), connect.NewRequest(&metarrv1.TaskServiceRunDirectoryScanRequest{
		ScannerSlug: "movies",
	}))
	if err == nil {
		t.Fatal("expected a rejection when no agent is mapped to the scan directory")
	}
	// 422 Unprocessable Entity maps to FailedPrecondition. The "no mapped
	// agent" guard stays; only the point-in-time online check was removed.
	if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
		t.Errorf("rejection code = %v, want FailedPrecondition", got)
	}
}
