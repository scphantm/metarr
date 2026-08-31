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

	"Metarr/internal/agent/nfo"
	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/eventbus"
)

// NFOReader answers the one synchronous call an HTTP request is waiting on:
// reading a single NFO file off this agent's disk. It reads the file and
// applies the path-traversal guards; the PubSubRouter it registers on owns
// the subscription loop and stamps source, correlation ID, and reply name on
// the answer.
//
// Pub/Sub rather than a stream because the caller is a browser: an answer
// that arrives after the request timed out is worthless, so durability would
// buy nothing and delivering to a disconnected agent should simply fail.
type NFOReader struct {
	config *ConfigStore
	logger *slog.Logger
	slug   string
}

// NewNFOReader returns an NFOReader for one agent.
func NewNFOReader(config *ConfigStore, logger *slog.Logger, slug string) *NFOReader {
	return &NFOReader{config: config, logger: logger, slug: slug}
}

// Register wires the NFO-read responder onto router. The router decodes the
// request envelope, calls the handler, and on a non-nil payload builds the
// reply — stamping this agent's source, the request's correlation ID, and
// eventbus.AgentNFOReadReplyEventName — then publishes it on the
// correlation-scoped reply channel.
func (r *NFOReader) Register(router *eventbus.PubSubRouter) {
	router.Respond(eventbus.AgentRequestChannel(r.slug), eventbus.AgentNFOReadReplyEventName,
		func(_ context.Context, request *eventbus.Event) ([]byte, error) {
			payload, err := json.Marshal(r.readNFO(request))
			if err != nil {
				r.logger.Error("could not encode NFO reply", "error", err)
				return nil, err
			}
			return payload, nil
		})
}

func (r *NFOReader) readNFO(request *eventbus.Event) agentproto.NFOReadReply {
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
