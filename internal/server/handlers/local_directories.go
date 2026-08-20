package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"Metarr/internal/server/mongostore"
	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/scanmodel"
)

const (
	// defaultLocalDirectoryLimit caps an unqualified listing, since a real
	// library holds thousands of directories.
	defaultLocalDirectoryLimit = 100
	maxLocalDirectoryLimit     = 500
)

// ListLocalDirectories handles GET /api/local-directories. It returns the
// directory records produced by directory scans, most recently scanned libraries
// included, filtered by type and scan root.
//
// @Summary		List scanned directories
// @Description	Returns the directory records in the local_directory collection, optionally filtered by media type and scan root path.
// @Tags			Media
// @Produce		json
// @Param			type		query		string	false	"Filter by media type: movie, tv or music_video"
// @Param			scan_root	query		string	false	"Filter by the configured scan directory these were found under"
// @Param			limit		query		int		false	"Maximum records to return (default 100, max 500)"
// @Param			skip		query		int		false	"Records to skip"
// @Success		200			{array}		scanmodel.TVSeries
// @Failure		400			{string}	string	"invalid type, limit or skip"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/local-directories [get]
func (h *Handlers) ListLocalDirectories(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	filter := mongostore.ListFilter{
		ScanRootPath: query.Get("scan_root"),
		Limit:        defaultLocalDirectoryLimit,
	}

	if rawType := query.Get("type"); rawType != "" {
		directoryType, err := scanmodel.ParseDirectoryType(rawType)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		filter.Type = directoryType
	}

	if rawLimit := query.Get("limit"); rawLimit != "" {
		limit, err := strconv.ParseInt(rawLimit, 10, 64)
		if err != nil || limit < 1 {
			http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
			return
		}
		if limit > maxLocalDirectoryLimit {
			limit = maxLocalDirectoryLimit
		}
		filter.Limit = limit
	}

	if rawSkip := query.Get("skip"); rawSkip != "" {
		skip, err := strconv.ParseInt(rawSkip, 10, 64)
		if err != nil || skip < 0 {
			http.Error(w, "skip must be zero or a positive integer", http.StatusBadRequest)
			return
		}
		filter.Skip = skip
	}

	directories, err := h.LocalDirectoryRepo.ListDirectories(r.Context(), filter)
	if err != nil {
		h.Logger.Error("failed to list local directories", "error", err)
		http.Error(w, "failed to list directories", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(directories); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// GetLocalDirectory handles GET /api/local-directories/{id}.
//
// @Summary		Fetch a scanned directory
// @Description	Returns one directory record by its generated id.
// @Tags			Media
// @Produce		json
// @Param			id	path		string	true	"Directory record id"
// @Success		200	{object}	scanmodel.TVSeries
// @Failure		400	{string}	string	"malformed id"
// @Failure		404	{string}	string	"no directory with that id"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/local-directories/{id} [get]
func (h *Handlers) GetLocalDirectory(w http.ResponseWriter, r *http.Request) {
	directoryID, ok := h.parseRecordID(w, r.PathValue("id"))
	if !ok {
		return
	}

	directory, err := h.LocalDirectoryRepo.GetDirectory(r.Context(), directoryID)
	if errors.Is(err, mongostore.ErrNotFound) {
		http.Error(w, "no directory with that id", http.StatusNotFound)
		return
	}
	if err != nil {
		h.Logger.Error("failed to fetch local directory", "id", directoryID.Hex(), "error", err)
		http.Error(w, "failed to fetch directory", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(directory); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// ListDirectoryMediaFiles handles
// GET /api/local-directories/{id}/media-files.
//
// @Summary		List a directory's media files
// @Description	Returns the media file records — the movie, episode or music video files themselves — belonging to one scanned directory.
// @Tags			Media
// @Produce		json
// @Param			id	path		string	true	"Directory record id"
// @Success		200	{array}		scanmodel.MediaFile
// @Failure		400	{string}	string	"malformed id"
// @Failure		404	{string}	string	"no directory with that id"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/local-directories/{id}/media-files [get]
func (h *Handlers) ListDirectoryMediaFiles(w http.ResponseWriter, r *http.Request) {
	directoryID, ok := h.parseRecordID(w, r.PathValue("id"))
	if !ok {
		return
	}

	// Confirm the directory exists so an unknown id is a 404 rather than an
	// empty list that looks like a directory with no media.
	if _, err := h.LocalDirectoryRepo.GetDirectory(r.Context(), directoryID); err != nil {
		h.writeDirectoryLookupError(w, directoryID, err)
		return
	}

	mediaFiles, err := h.LocalDirectoryRepo.ListMediaFiles(r.Context(), directoryID)
	if err != nil {
		h.Logger.Error("failed to list media files", "directory_id", directoryID.Hex(), "error", err)
		http.Error(w, "failed to list media files", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(mediaFiles); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// GetMediaFile handles GET /api/media-files/{id}.
//
// @Summary		Fetch a media file record
// @Description	Returns one media file record by its generated id, including its own NFO metadata, subtitles, artwork and episode ids.
// @Tags			Media
// @Produce		json
// @Param			id	path		string	true	"Media file record id"
// @Success		200	{object}	scanmodel.MediaFile
// @Failure		400	{string}	string	"malformed id"
// @Failure		404	{string}	string	"no media file with that id"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/media-files/{id} [get]
func (h *Handlers) GetMediaFile(w http.ResponseWriter, r *http.Request) {
	mediaFileID, ok := h.parseRecordID(w, r.PathValue("id"))
	if !ok {
		return
	}

	mediaFile, err := h.LocalDirectoryRepo.GetMediaFile(r.Context(), mediaFileID)
	if errors.Is(err, mongostore.ErrNotFound) {
		http.Error(w, "no media file with that id", http.StatusNotFound)
		return
	}
	if err != nil {
		h.Logger.Error("failed to fetch media file", "id", mediaFileID.Hex(), "error", err)
		http.Error(w, "failed to fetch media file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(mediaFile); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// GetLocalDirectoryNFO handles
// GET /api/local-directories/{id}/nfo?path=<relative path>. It reads and parses
// one NFO file live from disk, which is deliberately different from the
// scan-time snapshot held in the directory record: this reflects what the file
// says right now.
//
// @Summary		Read an NFO file from disk
// @Description	Reads and parses one .nfo file inside a scanned directory, live from disk rather than from the scan snapshot. The path is relative to the directory and may not escape it.
// @Tags			Media
// @Produce		json
// @Param			id		path		string	true	"Directory record id"
// @Param			path	query		string	true	"Path to the .nfo file, relative to the directory"
// @Success		200		{object}	metadata.Metadata
// @Failure		400		{string}	string	"missing or unsafe path, or not an .nfo file"
// @Failure		404		{string}	string	"no directory with that id, or no such file"
// @Failure		422		{string}	string	"the file could not be parsed"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/local-directories/{id}/nfo [get]
func (h *Handlers) GetLocalDirectoryNFO(w http.ResponseWriter, r *http.Request) {
	directoryID, ok := h.parseRecordID(w, r.PathValue("id"))
	if !ok {
		return
	}

	requestedPath := r.URL.Query().Get("path")
	if requestedPath == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	directory, err := h.LocalDirectoryRepo.GetDirectory(ctx, directoryID)
	if err != nil {
		h.writeDirectoryLookupError(w, directoryID, err)
		return
	}

	// The file lives on whichever agent scanned it, not here — the server has
	// no library mounted. Both the owning agent and the paths to send it are
	// derived from the scan root the record was stored under.
	agent, scannerSlug, relativeDirectory, err := h.locateOnAgent(directory.ScanRootPath, directory.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	payload, err := json.Marshal(agentproto.NFOReadRequest{
		ScannerSlug:       scannerSlug,
		RelativeDirectory: relativeDirectory,
		RelativePath:      requestedPath,
	})
	if err != nil {
		h.Logger.Error("failed to encode NFO request", "error", err)
		http.Error(w, "failed to read that NFO file", http.StatusInternalServerError)
		return
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, h.HeartbeatTimeout)
	defer cancel()

	reply, err := h.PubSub.Request(timeoutCtx, agentproto.RequestChannel(agent), eventbus.Event{
		CorrelationID: correlation.FromContext(ctx),
		Name:          agentproto.NFOReadEventName,
		Payload:       payload,
		Timestamp:     time.Now().UTC(),
	})
	if err != nil {
		// A timeout here means the agent is not answering, which is a different
		// problem from the file being unreadable and deserves its own status.
		if errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, "agent "+agent+" did not respond", http.StatusGatewayTimeout)
			return
		}
		h.Logger.Error("NFO request to agent failed", "agent", agent, "error", err)
		http.Error(w, "could not reach the agent holding this file", http.StatusBadGateway)
		return
	}

	var body agentproto.NFOReadReply
	if err := json.Unmarshal(reply.Payload, &body); err != nil {
		h.Logger.Error("malformed NFO reply from agent", "agent", agent, "error", err)
		http.Error(w, "could not read that NFO file", http.StatusBadGateway)
		return
	}
	if body.NotFound {
		http.Error(w, "no such file in this directory", http.StatusNotFound)
		return
	}
	if body.Error != "" {
		http.Error(w, "could not read that NFO file: "+body.Error, http.StatusUnprocessableEntity)
		return
	}

	document := body.Metadata

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(document); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// parseRecordID converts a path parameter into a record id, writing a 400 when
// it is not a valid identifier.
func (h *Handlers) parseRecordID(w http.ResponseWriter, rawID string) (bson.ObjectID, bool) {
	recordID, err := bson.ObjectIDFromHex(rawID)
	if err != nil {
		http.Error(w, "malformed id", http.StatusBadRequest)
		return bson.NilObjectID, false
	}
	return recordID, true
}

func (h *Handlers) writeDirectoryLookupError(w http.ResponseWriter, directoryID bson.ObjectID, err error) {
	if errors.Is(err, mongostore.ErrNotFound) {
		http.Error(w, "no directory with that id", http.StatusNotFound)
		return
	}
	h.Logger.Error("failed to fetch local directory", "id", directoryID.Hex(), "error", err)
	http.Error(w, "failed to fetch directory", http.StatusInternalServerError)
}

// locateOnAgent works out which agent holds a scanned directory and how to
// describe it in that agent's terms.
//
// Records are stored under the server's canonical paths, so the scan root
// identifies the library and the remainder is the part that means the same
// thing on both machines. Only relative paths are sent: an absolute one would
// be the server's, which does not exist on the agent.
func (h *Handlers) locateOnAgent(scanRootPath, directoryPath string) (agent, scannerSlug, relativeDirectory string, err error) {
	config := appconfig.Get()

	index := -1
	for i, scanDirectory := range config.DirectoryScanner.ScanDirectories {
		if sameScanRoot(scanDirectory.Directory, scanRootPath) {
			index = i
			break
		}
	}
	if index < 0 {
		return "", "", "", fmt.Errorf("this directory is not under any configured scan directory")
	}
	scannerSlug = config.DirectoryScanner.ScanDirectories[index].ScannerSlug

	owner, mapped := config.AgentForScanner(scannerSlug)
	if !mapped {
		return "", "", "", fmt.Errorf("no agent is mapped to scan directory %q", scannerSlug)
	}

	relative, err := filepath.Rel(filepath.Clean(scanRootPath), filepath.Clean(directoryPath))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", "", fmt.Errorf("this directory is not inside its own scan root")
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
