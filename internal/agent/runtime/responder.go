package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"Metarr/internal/agent/nfo"
	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/eventbus"
)

// Responder answers the synchronous calls an HTTP request is waiting on.
//
// These use Pub/Sub rather than a stream because the caller is a browser: an
// answer that arrives after the request timed out is worthless, so durability
// would buy nothing and delivering to a disconnected agent should simply fail.
type Responder struct {
	bus    *eventbus.PubSubBus
	config *ConfigStore
	logger *slog.Logger
	slug   string
}

// NewResponder returns a Responder for one agent.
func NewResponder(bus *eventbus.PubSubBus, config *ConfigStore, logger *slog.Logger, slug string) *Responder {
	return &Responder{bus: bus, config: config, logger: logger, slug: slug}
}

// Run answers requests until ctx is cancelled.
func (r *Responder) Run(ctx context.Context) {
	subscription := r.bus.Subscribe(ctx, eventbus.AgentRequestChannel(r.slug))
	defer func() { _ = subscription.Close() }()

	for message := range subscription.Channel() {
		var request eventbus.Event
		if err := json.Unmarshal([]byte(message.Payload), &request); err != nil {
			r.logger.Error("could not decode agent request", "error", err)
			continue
		}

		switch request.Name {
		case eventbus.AgentNFOReadEventName:
			r.replyNFORead(ctx, request)
		default:
			r.logger.Warn("ignoring unknown request", "event", request.Name)
		}
	}
}

func (r *Responder) replyNFORead(ctx context.Context, request eventbus.Event) {
	reply := r.readNFO(request)

	payload, err := json.Marshal(reply)
	if err != nil {
		r.logger.Error("could not encode NFO reply", "error", err)
		return
	}

	err = r.bus.Reply(ctx, request.CorrelationID, eventbus.Event{
		CorrelationID: request.CorrelationID,
		Name:          eventbus.AgentNFOReadEventName,
		Payload:       payload,
		Timestamp:     time.Now().UTC(),
	})
	if err != nil {
		r.logger.Error("could not send NFO reply", "error", err)
	}
}

func (r *Responder) readNFO(request eventbus.Event) agentproto.NFOReadReply {
	var body agentproto.NFOReadRequest
	if err := json.Unmarshal(request.Payload, &body); err != nil {
		return agentproto.NFOReadReply{Error: "malformed request"}
	}

	projection := r.config.Current()
	if projection == nil {
		return agentproto.NFOReadReply{Error: "this agent has no configuration yet"}
	}

	mapped, ok := agentproto.FindDirectory(projection, body.ScannerSlug)
	if !ok {
		return agentproto.NFOReadReply{
			Error: fmt.Sprintf("%q is not mapped to this agent", body.ScannerSlug),
		}
	}

	// Both hops are containment-checked against the root this agent was given.
	// The first stops a crafted directory from walking out of the library; the
	// second stops the filename from doing the same.
	agentDirectory, err := resolveUnder(mapped.AgentPath, body.RelativeDirectory)
	if err != nil {
		return agentproto.NFOReadReply{Error: err.Error()}
	}

	absolutePath, err := resolveWithinDirectory(agentDirectory, body.RelativePath)
	if err != nil {
		return agentproto.NFOReadReply{Error: err.Error()}
	}

	metadata, err := nfo.ReadFile(absolutePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return agentproto.NFOReadReply{NotFound: true, Error: "no such file"}
		}
		return agentproto.NFOReadReply{Error: err.Error()}
	}

	return agentproto.NFOReadReply{Metadata: metadata}
}

// resolveUnder joins a relative path onto root and refuses anything that
// escapes it. An empty relative path resolves to the root itself.
func resolveUnder(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", errors.New("directory must be relative to the library root")
	}

	cleanedRoot := filepath.Clean(root)
	candidate := filepath.Clean(filepath.Join(cleanedRoot, filepath.FromSlash(relative)))

	if !isWithin(cleanedRoot, candidate) {
		return "", fmt.Errorf("%q is not inside the library mapped for this agent", relative)
	}
	return candidate, nil
}

// isWithin reports whether candidate is root or sits beneath it.
func isWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// resolveWithinDirectory joins a caller-supplied relative path onto a scanned
// directory and refuses anything that escapes it.
//
// This is the security boundary for the only path that reads an arbitrary file
// from disk. Without it, a path of "../../../../etc/passwd" would turn a media
// metadata endpoint into an arbitrary file read — and it now runs on the machine
// that actually holds the library, so the stakes went up rather than down when
// it moved here from the server.
func resolveWithinDirectory(directoryPath, requestedPath string) (string, error) {
	if filepath.IsAbs(requestedPath) {
		return "", errors.New("path must be relative to the directory")
	}
	if !strings.EqualFold(filepath.Ext(requestedPath), ".nfo") {
		return "", errors.New("path must name an .nfo file")
	}

	cleanedDirectory := filepath.Clean(directoryPath)
	candidate := filepath.Clean(filepath.Join(cleanedDirectory, filepath.FromSlash(requestedPath)))

	if !isWithin(cleanedDirectory, candidate) {
		return "", errors.New("path is not inside the directory")
	}

	return candidate, nil
}
