package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"Metarr/internal/correlation"
	"Metarr/internal/eventbus"
	"Metarr/internal/mediascan"
)

type TaskRequest struct {
	Command string `json:"command"`
}

// SonarrCacheData handles POST /api/tasks/sonarr_cache_data. It fires the
// sonarr_cache_data event onto the event bus in a non-blocking way (the
// XAdd call returns as soon as the event is durably queued on the stream)
// and returns to the client immediately — the actual work happens
// asynchronously in the SonarrCacheData listener.
//
// @Summary		Trigger the sonarr_cache_data background job
// @Description	Fires the sonarr_cache_data event onto the durable event stream in a non-blocking way and returns as soon as the event has been queued.
// @Tags			Tasks
// @Accept			json
// @Produce		json
// @Param			request	body		TaskRequest	true	"Command to run"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body or unsupported command"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/tasks/sonarr_cache_data [post]
func (h *Handlers) SonarrCacheData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var req TaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Command != "run" {
		http.Error(w, `unsupported command, expected "run"`, http.StatusBadRequest)
		return
	}

	event := eventbus.Event{
		CorrelationID: correlationID,
		Name:          eventbus.SonarrCacheDataEventName,
		Timestamp:     time.Now().UTC(),
	}

	if err := h.Streams.Fire(ctx, eventbus.SonarrCacheDataStream, event); err != nil {
		h.Logger.Error("failed to fire sonarr_cache_data event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue task", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(AcceptedResponse{
		Status:        "accepted",
		Event:         eventbus.SonarrCacheDataEventName,
		CorrelationID: correlationID,
	})
}

// DirectoryScan handles POST /api/tasks/directory-scan/{slug}. It resolves the
// configured scan directory, fires the directory_scan event with that entry as
// its payload, and returns immediately — the filesystem walk happens
// asynchronously in the DirectoryScan listener.
//
// The resolved entry travels in the payload rather than just its slug, so the
// scan operates on the configuration as it stood when the scan was requested
// even if the config is edited before the listener picks the event up.
//
// @Summary		Trigger a directory scan
// @Description	Scans the configured scan directory with the given scanner_slug, storing one record per media item directory plus one per media file in the local_directory collection. Fires the directory_scan event onto the durable event stream and returns as soon as it has been queued.
// @Tags			Tasks
// @Accept			json
// @Produce		json
// @Param			slug	path		string		true	"scanner_slug of the scan directory to scan"
// @Param			request	body		TaskRequest	true	"Command to run"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body, unsupported command, or the entry has an unusable scan_type"
// @Failure		404		{string}	string	"no scan directory with that slug"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/tasks/directory-scan/{slug} [post]
func (h *Handlers) DirectoryScan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)
	slug := r.PathValue("slug")

	var req TaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Command != "run" {
		http.Error(w, `unsupported command, expected "run"`, http.StatusBadRequest)
		return
	}

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	index := appConfig.DirectoryScanner.FindScanDirectoryIndex(slug)
	if index == -1 {
		http.Error(w, "no scan directory with that slug", http.StatusNotFound)
		return
	}
	scanDirectory := appConfig.DirectoryScanner.ScanDirectories[index]

	if _, err := mediascan.ParseDirectoryType(scanDirectory.ScanType); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	payload, err := json.Marshal(scanDirectory)
	if err != nil {
		h.Logger.Error("failed to encode directory_scan payload", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue task", http.StatusInternalServerError)
		return
	}

	event := eventbus.Event{
		CorrelationID: correlationID,
		Name:          eventbus.DirectoryScanEventName,
		Payload:       payload,
		Timestamp:     time.Now().UTC(),
	}

	if err := h.Streams.Fire(ctx, eventbus.DirectoryScanStream, event); err != nil {
		h.Logger.Error("failed to fire directory_scan event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue task", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(AcceptedResponse{
		Status:        "accepted",
		Event:         eventbus.DirectoryScanEventName,
		CorrelationID: correlationID,
	})
}
