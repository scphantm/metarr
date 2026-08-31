package services

import (
	"context"
	"log/slog"
	"testing"

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

	bus, err := eventbus.NewStreamBus(client, eventbus.DefaultBusPolicy().Retention, eventbus.NewSlogAdapter(slog.Default()))
	if err != nil {
		t.Fatalf("NewStreamBus: %v", err)
	}
	return &TaskServer{Handlers: &handlers.Handlers{Streams: bus, Logger: slog.Default()}}, client
}

func configWithMappedAgent() *appconfig.Config {
	return &appconfig.Config{
		DirectoryScanner: &appconfig.DirectoryScannerConfig{
			ScanDirectories: []*appconfig.ScanDirectory{
				{ScannerSlug: "movies", ScanType: "movie", Directory: "/media/movies"},
			},
		},
		Agents: []*appconfig.AgentConfig{{
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
