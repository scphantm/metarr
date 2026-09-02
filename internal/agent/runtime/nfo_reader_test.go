package runtime

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/eventbus"
)

// These two functions are the only thing standing between a metadata endpoint
// and an arbitrary file read on the machine that holds the library. They run on
// the agent now, where the filesystem they guard is the real one, so the escape
// cases matter more here than they did on the server.

func TestResolveUnderAcceptsPathsInsideTheRoot(t *testing.T) {
	root := filepath.FromSlash("/mnt/tank/movies")

	cases := map[string]string{
		"":                      root,
		"Blade Runner (1982)":   filepath.Join(root, "Blade Runner (1982)"),
		"Show/Season 01":        filepath.Join(root, "Show", "Season 01"),
		"./Blade Runner (1982)": filepath.Join(root, "Blade Runner (1982)"),
		"Show/../Other":         filepath.Join(root, "Other"),
	}

	for relative, want := range cases {
		got, err := resolveUnder(root, relative)
		if err != nil {
			t.Errorf("resolveUnder(%q) error = %v", relative, err)
			continue
		}
		if got != want {
			t.Errorf("resolveUnder(%q) = %q, want %q", relative, got, want)
		}
	}
}

func TestResolveUnderRejectsEscapes(t *testing.T) {
	root := filepath.FromSlash("/mnt/tank/movies")

	for _, relative := range []string{
		"..",
		"../etc",
		"../../../../etc",
		"Show/../../../etc",
		filepath.FromSlash("/etc/passwd"),
	} {
		if got, err := resolveUnder(root, relative); err == nil {
			t.Errorf("resolveUnder(%q) = %q, want an error", relative, got)
		}
	}
}

func TestResolveWithinDirectoryAcceptsNFOFiles(t *testing.T) {
	directory := filepath.FromSlash("/mnt/tank/movies/Blade Runner (1982)")

	for _, name := range []string{"movie.nfo", "Movie.NFO", "extras/behind.nfo"} {
		got, err := resolveWithinDirectory(directory, name)
		if err != nil {
			t.Errorf("resolveWithinDirectory(%q) error = %v", name, err)
			continue
		}
		if want := filepath.Join(directory, filepath.FromSlash(name)); got != want {
			t.Errorf("resolveWithinDirectory(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestResolveWithinDirectoryRejectsEscapesAndNonNFO(t *testing.T) {
	directory := filepath.FromSlash("/mnt/tank/movies/Blade Runner (1982)")

	cases := map[string]string{
		"absolute path":        filepath.FromSlash("/etc/passwd"),
		"absolute nfo":         filepath.FromSlash("/etc/passwd.nfo"),
		"traversal":            "../../../../etc/passwd.nfo",
		"traversal mid-path":   "extras/../../../secrets.nfo",
		"not an nfo":           "movie.mkv",
		"no extension":         "movie",
		"nfo-lookalike suffix": "movie.nfo.txt",
	}

	for name, requested := range cases {
		if got, err := resolveWithinDirectory(directory, requested); err == nil {
			t.Errorf("%s: resolveWithinDirectory(%q) = %q, want an error", name, requested, got)
		}
	}
}

// A directory NFO read reaches the agent and returns through the Bus: the
// responder registered by NFOReader.Register answers a bus.Request on the
// agent's request/reply topic, and the reply carries the topic's reply event
// name. With no projection installed the reader's answer is a structured
// "not configured yet" error — which is exactly what proves the request got
// to the handler and a reply came back.
func TestNFOReaderRegisterAnswersThroughTheBus(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	const slug = "nas-01"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	configStore := NewConfigStore(client, logger, slug, nil)
	reader := NewNFOReader(configStore, logger, slug)

	bus, err := eventbus.New(eventbus.Config{
		Redis:   client,
		Source:  eventbus.AgentSource(slug),
		Streams: eventbus.ChannelStreamTransport(),
		Policy:  eventbus.DefaultBusPolicy,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("eventbus.New: %v", err)
	}
	if err := reader.Register(bus); err != nil {
		t.Fatalf("reader.Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- bus.Run(ctx) }()
	select {
	case <-bus.Ready():
	case err := <-runDone:
		t.Fatalf("bus stopped before ready: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("bus never became ready")
	}
	t.Cleanup(func() {
		cancel()
		<-runDone
		_ = bus.Close()
	})

	payload, err := json.Marshal(agentproto.NFOReadRequest{
		ScannerSlug:       "movies",
		RelativeDirectory: "Blade Runner (1982)",
		RelativePath:      "movie.nfo",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	reqCtx, reqCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer reqCancel()

	reply, err := bus.Request(reqCtx, eventbus.AgentRequestTopic(slug),
		eventbus.AgentNFOReadEventName, "corr-nfo", payload)
	if err != nil {
		t.Fatalf("bus.Request: %v", err)
	}
	if reply.GetName() != eventbus.AgentNFOReadReplyEventName {
		t.Errorf("reply name = %q, want %q", reply.GetName(), eventbus.AgentNFOReadReplyEventName)
	}

	var body agentproto.NFOReadReply
	if err := json.Unmarshal(reply.GetPayload(), &body); err != nil {
		t.Fatalf("unmarshal reply: %v", err)
	}
	if body.Error == "" {
		t.Error("expected a structured error in the reply (no projection installed)")
	}
}

func TestIsWithinTreatsTheRootItselfAsInside(t *testing.T) {
	root := filepath.FromSlash("/mnt/tank/movies")

	if !isWithin(root, root) {
		t.Error("a root is not reported as inside itself")
	}
	if !isWithin(root, filepath.Join(root, "a", "b")) {
		t.Error("a nested path is not reported as inside")
	}
	// A sibling whose name merely starts with the root's must not pass.
	if isWithin(root, filepath.FromSlash("/mnt/tank/movies-backup/x")) {
		t.Error("a sibling sharing a name prefix was reported as inside")
	}
}
