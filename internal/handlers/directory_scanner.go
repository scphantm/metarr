package handlers

import (
	"encoding/json"
	"net/http"

	"Metarr/internal/appconfig"
	"Metarr/internal/correlation"
	"Metarr/internal/eventbus"
)

// GetDirectoryScannerConfig handles GET /api/config/directory-scanner. It
// reads the application config straight from MongoDB and returns the
// directory scanner section.
//
// @Summary		Fetch the directory scanner config
// @Description	Reads the application config from MongoDB and returns the directory scanner section (parallel_count and scan_directories).
// @Tags			Config
// @Produce		json
// @Success		200	{object}	appconfig.DirectoryScannerConfig
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner [get]
func (h *Handlers) GetDirectoryScannerConfig(w http.ResponseWriter, r *http.Request) {
	appConfig, err := h.AppConfigRepo.Get(r.Context())
	if err != nil {
		h.Logger.Error("failed to fetch app config", "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(appConfig.DirectoryScanner)
}

// UpdateDirectoryScannerRequest is a partial update: only ParallelCount, if
// present, is changed. scan_directories is managed through the dedicated
// /api/config/directory-scanner/directories endpoints instead.
type UpdateDirectoryScannerRequest struct {
	ParallelCount *int `json:"parallel_count,omitempty"`
}

// UpdateDirectoryScannerConfig handles PUT /api/config/directory-scanner. It
// fires the system_config_update event with the updated document as its
// payload and returns to the client as soon as the event is fired, exactly
// like UpdateConfig.
//
// @Summary		Update the directory scanner config
// @Description	Updates parallel_count. A provided value must be greater than zero.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			request	body		UpdateDirectoryScannerRequest	true	"Fields to update"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body, or parallel_count was not greater than zero"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner [put]
func (h *Handlers) UpdateDirectoryScannerConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var req UpdateDirectoryScannerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ParallelCount != nil && *req.ParallelCount <= 0 {
		http.Error(w, "parallel_count must be greater than zero", http.StatusBadRequest)
		return
	}

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	if req.ParallelCount != nil {
		appConfig.DirectoryScanner.ParallelCount = *req.ParallelCount
	}

	if err := h.fireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		h.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue config update", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(AcceptedResponse{
		Status:        "accepted",
		Event:         eventbus.SystemConfigUpdateEventName,
		CorrelationID: correlationID,
	})
}

// ListScanDirectories handles GET /api/config/directory-scanner/directories.
// It reads the application config straight from MongoDB and returns every
// configured scan directory.
//
// @Summary		List scan directories
// @Description	Reads the application config from MongoDB and returns every configured scan directory.
// @Tags			Config
// @Produce		json
// @Success		200	{array}	appconfig.ScanDirectory
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner/directories [get]
func (h *Handlers) ListScanDirectories(w http.ResponseWriter, r *http.Request) {
	appConfig, err := h.AppConfigRepo.Get(r.Context())
	if err != nil {
		h.Logger.Error("failed to fetch app config", "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(appConfig.DirectoryScanner.ScanDirectories)
}

// GetScanDirectory handles
// GET /api/config/directory-scanner/directories/{slug}.
//
// @Summary		Fetch a single scan directory
// @Description	Reads the application config from MongoDB and returns the scan directory instance with the given scanner_slug.
// @Tags			Config
// @Produce		json
// @Param			slug	path		string	true	"scanner_slug"
// @Success		200		{object}	appconfig.ScanDirectory
// @Failure		404		{string}	string	"no scan directory with that slug"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner/directories/{slug} [get]
func (h *Handlers) GetScanDirectory(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	appConfig, err := h.AppConfigRepo.Get(r.Context())
	if err != nil {
		h.Logger.Error("failed to fetch app config", "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	index := appConfig.DirectoryScanner.FindScanDirectoryIndex(slug)
	if index == -1 {
		http.Error(w, "no scan directory with that slug", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(appConfig.DirectoryScanner.ScanDirectories[index])
}

// CreateScanDirectory handles
// POST /api/config/directory-scanner/directories. It validates scanner_slug
// is present and unique, appends the new entry, and fires
// system_config_update with the full resulting document.
//
// @Summary		Add a scan directory
// @Description	Adds a new scan directory. scanner_slug is required and must be unique.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			request	body		appconfig.ScanDirectory	true	"New scan directory"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body or missing scanner_slug"
// @Failure		409		{string}	string	"scanner_slug already in use"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner/directories [post]
func (h *Handlers) CreateScanDirectory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var entry appconfig.ScanDirectory
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if entry.ScannerSlug == "" {
		http.Error(w, "scanner_slug is required", http.StatusBadRequest)
		return
	}

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	if appConfig.DirectoryScanner.FindScanDirectoryIndex(entry.ScannerSlug) != -1 {
		http.Error(w, "scanner_slug already in use", http.StatusConflict)
		return
	}

	appConfig.DirectoryScanner.ScanDirectories = append(appConfig.DirectoryScanner.ScanDirectories, entry)

	if err := h.fireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		h.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue config update", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(AcceptedResponse{
		Status:        "accepted",
		Event:         eventbus.SystemConfigUpdateEventName,
		CorrelationID: correlationID,
	})
}

// UpdateScanDirectory handles
// PUT /api/config/directory-scanner/directories/{slug}. scanner_slug can
// never be changed once set: if the body sets a different, non-empty
// scanner_slug than the URL, the request is rejected. Every other field is
// replaced with what the body supplies.
//
// @Summary		Update a scan directory
// @Description	Replaces every field of the scan directory at the given scanner_slug except scanner_slug itself, which cannot be changed once set.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			slug	path		string					true	"scanner_slug"
// @Param			request	body		appconfig.ScanDirectory	true	"Updated scan directory"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body, or attempted to change scanner_slug"
// @Failure		404		{string}	string	"no scan directory with that slug"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner/directories/{slug} [put]
func (h *Handlers) UpdateScanDirectory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)
	slug := r.PathValue("slug")

	var entry appconfig.ScanDirectory
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if entry.ScannerSlug != "" && entry.ScannerSlug != slug {
		http.Error(w, "scanner_slug cannot be changed", http.StatusBadRequest)
		return
	}
	entry.ScannerSlug = slug

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
	appConfig.DirectoryScanner.ScanDirectories[index] = entry

	if err := h.fireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		h.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue config update", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(AcceptedResponse{
		Status:        "accepted",
		Event:         eventbus.SystemConfigUpdateEventName,
		CorrelationID: correlationID,
	})
}

// DeleteScanDirectory handles
// DELETE /api/config/directory-scanner/directories/{slug}.
//
// @Summary		Delete a scan directory
// @Description	Removes the scan directory with the given scanner_slug and fires system_config_update with the resulting document.
// @Tags			Config
// @Produce		json
// @Param			slug	path		string	true	"scanner_slug"
// @Success		202		{object}	AcceptedResponse
// @Failure		404		{string}	string	"no scan directory with that slug"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner/directories/{slug} [delete]
func (h *Handlers) DeleteScanDirectory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)
	slug := r.PathValue("slug")

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
	scanDirectories := appConfig.DirectoryScanner.ScanDirectories
	appConfig.DirectoryScanner.ScanDirectories = append(scanDirectories[:index], scanDirectories[index+1:]...)

	if err := h.fireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		h.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue config update", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(AcceptedResponse{
		Status:        "accepted",
		Event:         eventbus.SystemConfigUpdateEventName,
		CorrelationID: correlationID,
	})
}
