package services

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	"go.mongodb.org/mongo-driver/v2/bson"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/scanmodel"
)

const (
	defaultLocalDirectoryLimit = 100
	maxLocalDirectoryLimit     = 500
)

// LocalDirectoryServer implements
// metarrv1connect.LocalDirectoryServiceHandler. Responses carry the generated
// scan record messages directly (scanmodel aliases them) — see docs/adr/0005.
type LocalDirectoryServer struct {
	*handlers.Handlers
}

// LocalDirectoryAuthPolicies is this service's method-name -> policy map.
// Mirrors every local-directories/media-files route in router.go being
// GroupTasks.
var LocalDirectoryAuthPolicies = map[string]httpserver.RPCPolicy{
	"ListDirectories": {Group: auth.GroupTasks, ReadOnly: true},
	"GetDirectory":    {Group: auth.GroupTasks, ReadOnly: true},
	"ListMediaFiles":  {Group: auth.GroupTasks, ReadOnly: true},
	"GetMediaFile":    {Group: auth.GroupTasks, ReadOnly: true},
	"GetDirectoryNFO": {Group: auth.GroupTasks, ReadOnly: true},
}

func (s *LocalDirectoryServer) ListDirectories(
	ctx context.Context,
	req *connect.Request[metarrv1.LocalDirectoryServiceListDirectoriesRequest],
) (*connect.Response[metarrv1.LocalDirectoryServiceListDirectoriesResponse], error) {
	filter := mongostore.ListFilter{
		ScanRootPath: req.Msg.GetScanRoot(),
		Limit:        defaultLocalDirectoryLimit,
	}

	if rawType := req.Msg.GetType(); rawType != "" {
		directoryType, err := scanmodel.ParseDirectoryType(rawType)
		if err != nil {
			return nil, connectError(http.StatusBadRequest, err)
		}
		filter.Type = directoryType
	}

	if limit := req.Msg.GetLimit(); limit != 0 {
		if limit < 1 {
			return nil, connectError(http.StatusBadRequest, errors.New("limit must be a positive integer"))
		}
		if limit > maxLocalDirectoryLimit {
			limit = maxLocalDirectoryLimit
		}
		filter.Limit = int64(limit)
	}

	if skip := req.Msg.GetSkip(); skip != 0 {
		if skip < 0 {
			return nil, connectError(http.StatusBadRequest, errors.New("skip must be zero or a positive integer"))
		}
		filter.Skip = int64(skip)
	}

	directories, err := s.LocalDirectoryRepo.ListDirectories(ctx, filter)
	if err != nil {
		s.Logger.Error("failed to list local directories", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to list directories"))
	}

	return connect.NewResponse(&metarrv1.LocalDirectoryServiceListDirectoriesResponse{Directories: directories}), nil
}

func (s *LocalDirectoryServer) GetDirectory(
	ctx context.Context,
	req *connect.Request[metarrv1.LocalDirectoryServiceGetDirectoryRequest],
) (*connect.Response[metarrv1.LocalDirectoryServiceGetDirectoryResponse], error) {
	directoryID, err := parseRecordID(req.Msg.GetId())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	directory, err := s.LocalDirectoryRepo.GetDirectory(ctx, directoryID)
	if err != nil {
		return nil, directoryLookupError(err, directoryID, s.Logger)
	}

	return connect.NewResponse(&metarrv1.LocalDirectoryServiceGetDirectoryResponse{Directory: directory}), nil
}

func (s *LocalDirectoryServer) ListMediaFiles(
	ctx context.Context,
	req *connect.Request[metarrv1.LocalDirectoryServiceListMediaFilesRequest],
) (*connect.Response[metarrv1.LocalDirectoryServiceListMediaFilesResponse], error) {
	directoryID, err := parseRecordID(req.Msg.GetDirectoryId())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	// Confirm the directory exists so an unknown id is a not-found rather than
	// an empty list that looks like a directory with no media.
	if _, err := s.LocalDirectoryRepo.GetDirectory(ctx, directoryID); err != nil {
		return nil, directoryLookupError(err, directoryID, s.Logger)
	}

	mediaFiles, err := s.LocalDirectoryRepo.ListMediaFiles(ctx, directoryID)
	if err != nil {
		s.Logger.Error("failed to list media files", "directory_id", directoryID.Hex(), "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to list media files"))
	}

	return connect.NewResponse(&metarrv1.LocalDirectoryServiceListMediaFilesResponse{MediaFiles: mediaFiles}), nil
}

func (s *LocalDirectoryServer) GetMediaFile(
	ctx context.Context,
	req *connect.Request[metarrv1.LocalDirectoryServiceGetMediaFileRequest],
) (*connect.Response[metarrv1.LocalDirectoryServiceGetMediaFileResponse], error) {
	mediaFileID, err := parseRecordID(req.Msg.GetId())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	mediaFile, err := s.LocalDirectoryRepo.GetMediaFile(ctx, mediaFileID)
	if errors.Is(err, mongostore.ErrNotFound) {
		return nil, connectError(http.StatusNotFound, errors.New("no media file with that id"))
	}
	if err != nil {
		s.Logger.Error("failed to fetch media file", "id", mediaFileID.Hex(), "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch media file"))
	}

	return connect.NewResponse(&metarrv1.LocalDirectoryServiceGetMediaFileResponse{MediaFile: mediaFile}), nil
}

func (s *LocalDirectoryServer) GetDirectoryNFO(
	ctx context.Context,
	req *connect.Request[metarrv1.LocalDirectoryServiceGetDirectoryNFORequest],
) (*connect.Response[metarrv1.LocalDirectoryServiceGetDirectoryNFOResponse], error) {
	directoryID, err := parseRecordID(req.Msg.GetDirectoryId())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	requestedPath := req.Msg.GetPath()
	if requestedPath == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("path is required"))
	}

	correlationID := correlation.FromContext(ctx)

	directory, err := s.LocalDirectoryRepo.GetDirectory(ctx, directoryID)
	if err != nil {
		return nil, directoryLookupError(err, directoryID, s.Logger)
	}

	// The file lives on whichever agent scanned it, not here — the server has
	// no library mounted. Both the owning agent and the paths to send it are
	// derived from the scan root the record was stored under.
	agent, scannerSlug, relativeDirectory, err := s.locateOnAgent(directory.ScanRootPath, directory.Path)
	if err != nil {
		return nil, connectError(http.StatusUnprocessableEntity, err)
	}

	payload, err := json.Marshal(agentproto.NFOReadRequest{
		ScannerSlug:       scannerSlug,
		RelativeDirectory: relativeDirectory,
		RelativePath:      requestedPath,
	})
	if err != nil {
		s.Logger.Error("failed to encode NFO request", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to read that NFO file"))
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, s.HeartbeatTimeout)
	defer cancel()

	reply, err := s.Bus.Request(timeoutCtx, eventbus.AgentRequestTopic(agent),
		eventbus.AgentNFOReadEventName, correlationID, payload)
	if err != nil {
		// No answer means the agent is not there, which is a different problem
		// from the file being unreadable and deserves its own status.
		if errors.Is(err, eventbus.ErrNoResponder) {
			return nil, connectError(http.StatusGatewayTimeout, errors.New("agent "+agent+" did not respond"))
		}
		s.Logger.Error("NFO request to agent failed", "agent", agent, "error", err)
		return nil, connectError(http.StatusBadGateway, errors.New("could not reach the agent holding this file"))
	}

	var body agentproto.NFOReadReply
	if err := json.Unmarshal(reply.Payload, &body); err != nil {
		s.Logger.Error("malformed NFO reply from agent", "agent", agent, "error", err)
		return nil, connectError(http.StatusBadGateway, errors.New("could not read that NFO file"))
	}
	if body.NotFound {
		return nil, connectError(http.StatusNotFound, errors.New("no such file in this directory"))
	}
	if body.Error != "" {
		return nil, connectError(http.StatusUnprocessableEntity, errors.New("could not read that NFO file: "+body.Error))
	}

	return connect.NewResponse(&metarrv1.LocalDirectoryServiceGetDirectoryNFOResponse{Metadata: body.Metadata}), nil
}

func parseRecordID(rawID string) (bson.ObjectID, error) {
	recordID, err := bson.ObjectIDFromHex(rawID)
	if err != nil {
		return bson.NilObjectID, errors.New("malformed id")
	}
	return recordID, nil
}

func directoryLookupError(err error, directoryID bson.ObjectID, logger *slog.Logger) error {
	if errors.Is(err, mongostore.ErrNotFound) {
		return connectError(http.StatusNotFound, errors.New("no directory with that id"))
	}
	logger.Error("failed to fetch local directory", "id", directoryID.Hex(), "error", err)
	return connectError(http.StatusInternalServerError, errors.New("failed to fetch directory"))
}

// locateOnAgent works out which agent holds a scanned directory and how to
// describe it in that agent's terms.
//
// Records are stored under the server's canonical paths, so the scan root
// identifies the library and the remainder is the part that means the same
// thing on both machines. Only relative paths are sent: an absolute one would
// be the server's, which does not exist on the agent.
func (s *LocalDirectoryServer) locateOnAgent(scanRootPath, directoryPath string) (agent, scannerSlug, relativeDirectory string, err error) {
	config := appconfig.Get()

	index := -1
	for i, scanDirectory := range config.DirectoryScanner.ScanDirectories {
		if sameScanRoot(scanDirectory.Directory, scanRootPath) {
			index = i
			break
		}
	}
	if index < 0 {
		return "", "", "", errors.New("this directory is not under any configured scan directory")
	}
	scannerSlug = config.DirectoryScanner.ScanDirectories[index].ScannerSlug

	owner, mapped := appconfig.AgentForScanner(config, scannerSlug)
	if !mapped {
		return "", "", "", errors.New("no agent is mapped to scan directory " + scannerSlug)
	}

	relative, err := filepath.Rel(filepath.Clean(scanRootPath), filepath.Clean(directoryPath))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", "", errors.New("this directory is not inside its own scan root")
	}
	if relative == "." {
		relative = ""
	}

	return owner.Slug, scannerSlug, relative, nil
}

// sameScanRoot reports whether a configured scan directory names the same root
// a record was stored under.
//
// A configured directory may be written relative ("./data/Shows") while stored
// records always carry an absolute path, because scanning resolves it before
// walking. Comparing the cleaned forms alone would miss that, so the absolute
// forms are compared as well.
func sameScanRoot(configured, stored string) bool {
	if filepath.Clean(configured) == filepath.Clean(stored) {
		return true
	}

	absoluteConfigured, err := filepath.Abs(configured)
	if err != nil {
		return false
	}
	absoluteStored, err := filepath.Abs(stored)
	if err != nil {
		return false
	}
	return absoluteConfigured == absoluteStored
}
