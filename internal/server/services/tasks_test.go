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

	resp, err := server.RunDirectoryScan(context.Background(), connect.NewRequest(&metarrv1.TaskServiceRunDirectoryScanRequest{
		ScannerSlug: "movies",
		Command:     "run",
	}))
	if err != nil {
		t.Fatalf("RunDirectoryScan returned an error for an absent agent: %v", err)
	}
	if resp.Msg.GetStatus() != "accepted" {
		t.Errorf("status = %q, want accepted", resp.Msg.GetStatus())
	}

	got, err := client.XLen(context.Background(), eventbus.AgentCommandStream("nas-01")).Result()
	if err != nil || got != 1 {
		t.Errorf("agent command stream length = %d (err %v), want 1 queued command", got, err)
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
		Command:     "run",
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
