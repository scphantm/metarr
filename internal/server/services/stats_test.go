package services

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/shared/eventbus"
)

// methodFor mirrors what the connect auth interceptor does with a policy: a
// read-only RPC is checked as a GET, everything else as a POST.
func methodFor(policy httpserver.RPCPolicy) string {
	if policy.ReadOnly {
		return http.MethodGet
	}
	return http.MethodPost
}

// The auth-policy map is the contract: Get and Stream are read-only reads,
// Purge is a write, and all three sit under the config-admin group.
func TestStatsAuthPolicies(t *testing.T) {
	for _, name := range []string{"Get", "Stream", "Purge"} {
		policy, ok := StatsAuthPolicies[name]
		if !ok {
			t.Fatalf("no auth policy registered for %q", name)
		}
		if policy.Group != auth.GroupConfig {
			t.Errorf("%s group = %q, want %q", name, policy.Group, auth.GroupConfig)
		}
	}

	if !StatsAuthPolicies["Get"].ReadOnly {
		t.Error("Get must be read-only")
	}
	if !StatsAuthPolicies["Stream"].ReadOnly {
		t.Error("Stream must be read-only")
	}
	if StatsAuthPolicies["Purge"].ReadOnly {
		t.Error("Purge trims streams — it must not be read-only")
	}
}

// A read-only config key is refused Purge (checked as a write), while an
// admin key is allowed it — the same check the connect auth interceptor
// applies from the policy map.
func TestStatsPurgeRefusesReadOnlyCaller(t *testing.T) {
	purge := StatsAuthPolicies["Purge"]

	if auth.Authorized(auth.RoleReadOnly, purge.Group, methodFor(purge)) {
		t.Error("a read-only caller must not be authorized for Purge")
	}
	if !auth.Authorized(auth.RoleAdmin, purge.Group, methodFor(purge)) {
		t.Error("an admin caller must be authorized for Purge")
	}
}

func newTestStatsServer(t *testing.T) (*StatsServer, *redis.Client, *bytes.Buffer) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return &StatsServer{Handlers: &handlers.Handlers{Redis: client, Logger: logger}}, client, logs
}

func seedPastEntry(t *testing.T, client *redis.Client, stream string, age time.Duration) {
	t.Helper()
	id := strconv.FormatInt(time.Now().Add(-age).UnixMilli(), 10) + "-0"
	if err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: stream, ID: id, Values: map[string]any{"payload": "{}"},
	}).Err(); err != nil {
		t.Fatalf("seed %s: %v", stream, err)
	}
}

// Purge of a single stream trims it, returns the dropped count, and writes
// one warn-level audit line naming the actor, the stream and the count.
func TestStatsPurgeSingleStream(t *testing.T) {
	server, client, logs := newTestStatsServer(t)
	ctx := auth.WithRole(context.Background(), auth.RoleAdmin)

	seedPastEntry(t, client, eventbus.AgentScanResultStream, 60*time.Second)
	seedPastEntry(t, client, eventbus.AgentScanResultStream, 45*time.Second)
	seedPastEntry(t, client, eventbus.AgentScanResultStream, 30*time.Second)

	resp, err := server.Purge(ctx, connect.NewRequest(&metarrv1.StatsServicePurgeRequest{
		Stream: eventbus.AgentScanResultStream,
	}))
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if len(resp.Msg.GetResults()) != 1 {
		t.Fatalf("results = %d, want 1", len(resp.Msg.GetResults()))
	}
	got := resp.Msg.GetResults()[0]
	if got.GetStream() != eventbus.AgentScanResultStream {
		t.Errorf("result stream = %q, want %q", got.GetStream(), eventbus.AgentScanResultStream)
	}
	if got.GetDropped() != 3 {
		t.Errorf("dropped = %d, want 3", got.GetDropped())
	}
	if n, _ := client.XLen(ctx, eventbus.AgentScanResultStream).Result(); n != 0 {
		t.Errorf("stream length after purge = %d, want 0", n)
	}

	entry := lastLogRecord(t, logs)
	if entry["level"] != "WARN" {
		t.Errorf("audit line level = %v, want WARN", entry["level"])
	}
	if entry["actor"] != string(auth.RoleAdmin) {
		t.Errorf("audit line actor = %v, want %q", entry["actor"], auth.RoleAdmin)
	}
	if entry["stream"] != eventbus.AgentScanResultStream {
		t.Errorf("audit line stream = %v, want %q", entry["stream"], eventbus.AgentScanResultStream)
	}
	if entry["dropped"].(float64) != 3 {
		t.Errorf("audit line dropped = %v, want 3", entry["dropped"])
	}
}

// Purge with all set clears every discovered durable stream and writes one
// audit line per stream.
func TestStatsPurgeAll(t *testing.T) {
	server, client, logs := newTestStatsServer(t)
	ctx := auth.WithRole(context.Background(), auth.RoleAdmin)

	seedPastEntry(t, client, eventbus.AgentScanResultStream, 60*time.Second)
	seedPastEntry(t, client, eventbus.AgentScanResultStream, 30*time.Second)
	seedPastEntry(t, client, eventbus.AgentCommandStream("nas-01"), 20*time.Second)
	// AgentNodeResultStream is never created — purge-all must not choke on it.

	resp, err := server.Purge(ctx, connect.NewRequest(&metarrv1.StatsServicePurgeRequest{All: true}))
	if err != nil {
		t.Fatalf("Purge all: %v", err)
	}

	dropped := map[string]int64{}
	for _, r := range resp.Msg.GetResults() {
		dropped[r.GetStream()] = r.GetDropped()
	}
	for _, want := range []string{
		eventbus.AgentScanResultStream,
		eventbus.AgentNodeResultStream,
		eventbus.AgentCommandStream("nas-01"),
	} {
		if _, ok := dropped[want]; !ok {
			t.Errorf("stream %q missing from purge-all results", want)
		}
	}
	if dropped[eventbus.AgentScanResultStream] != 2 {
		t.Errorf("%s dropped = %d, want 2", eventbus.AgentScanResultStream, dropped[eventbus.AgentScanResultStream])
	}

	if lines := strings.Count(strings.TrimSpace(logs.String()), "\n") + 1; lines != len(resp.Msg.GetResults()) {
		t.Errorf("audit lines = %d, want one per purged stream (%d)", lines, len(resp.Msg.GetResults()))
	}
}

// Exactly one of stream / all must be given.
func TestStatsPurgeRejectsAmbiguousRequest(t *testing.T) {
	server, _, _ := newTestStatsServer(t)
	ctx := auth.WithRole(context.Background(), auth.RoleAdmin)

	cases := map[string]*metarrv1.StatsServicePurgeRequest{
		"neither stream nor all": {},
		"both stream and all":    {Stream: eventbus.AgentScanResultStream, All: true},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := server.Purge(ctx, connect.NewRequest(req))
			if err == nil {
				t.Fatal("expected an error")
			}
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
			}
		})
	}
}

func lastLogRecord(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("no log records captured")
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &entry); err != nil {
		t.Fatalf("parse log record %q: %v", lines[len(lines)-1], err)
	}
	return entry
}
