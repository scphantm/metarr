package handlers

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"Metarr/internal/mediascan"
	"Metarr/internal/mongostore"
	"Metarr/internal/nfo"
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
// @Success		200			{array}		mediascan.TVSeries
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
		directoryType, err := mediascan.ParseDirectoryType(rawType)
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
	json.NewEncoder(w).Encode(directories)
}

// GetLocalDirectory handles GET /api/local-directories/{id}.
//
// @Summary		Fetch a scanned directory
// @Description	Returns one directory record by its generated id.
// @Tags			Media
// @Produce		json
// @Param			id	path		string	true	"Directory record id"
// @Success		200	{object}	mediascan.TVSeries
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
	json.NewEncoder(w).Encode(directory)
}

// ListDirectoryMediaFiles handles
// GET /api/local-directories/{id}/media-files.
//
// @Summary		List a directory's media files
// @Description	Returns the media file records — the movie, episode or music video files themselves — belonging to one scanned directory.
// @Tags			Media
// @Produce		json
// @Param			id	path		string	true	"Directory record id"
// @Success		200	{array}		mediascan.MediaFile
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
	json.NewEncoder(w).Encode(mediaFiles)
}

// GetMediaFile handles GET /api/media-files/{id}.
//
// @Summary		Fetch a media file record
// @Description	Returns one media file record by its generated id, including its own NFO metadata, subtitles, artwork and episode ids.
// @Tags			Media
// @Produce		json
// @Param			id	path		string	true	"Media file record id"
// @Success		200	{object}	mediascan.MediaFile
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
	json.NewEncoder(w).Encode(mediaFile)
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

	directory, err := h.LocalDirectoryRepo.GetDirectory(r.Context(), directoryID)
	if err != nil {
		h.writeDirectoryLookupError(w, directoryID, err)
		return
	}

	absolutePath, err := resolveWithinDirectory(directory.Path, requestedPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	document, err := nfo.ReadFile(absolutePath)
	if err != nil {
		// A file that isn't there is a client mistake; anything else means the
		// file exists but could not be understood.
		if errors.Is(err, fs.ErrNotExist) {
			http.Error(w, "no such file in this directory", http.StatusNotFound)
			return
		}
		http.Error(w, "could not read that NFO file: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(document)
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

// resolveWithinDirectory joins a caller-supplied relative path onto a scanned
// directory and refuses anything that escapes it.
//
// This is the security boundary for the only endpoint that reads arbitrary files
// from disk. Without it, a path of "../../../../etc/passwd" would turn a media
// metadata endpoint into an arbitrary file read.
func resolveWithinDirectory(directoryPath, requestedPath string) (string, error) {
	if filepath.IsAbs(requestedPath) {
		return "", errors.New("path must be relative to the directory")
	}
	if !strings.EqualFold(filepath.Ext(requestedPath), ".nfo") {
		return "", errors.New("path must name an .nfo file")
	}

	cleanedDirectory := filepath.Clean(directoryPath)
	candidate := filepath.Clean(filepath.Join(cleanedDirectory, filepath.FromSlash(requestedPath)))

	relative, err := filepath.Rel(cleanedDirectory, candidate)
	if err != nil {
		return "", errors.New("path is not inside the directory")
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("path is not inside the directory")
	}

	return candidate, nil
}
