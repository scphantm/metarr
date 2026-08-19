package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"Metarr/internal/appconfig"
	"Metarr/internal/correlation"
	"Metarr/internal/eventbus"
	"Metarr/internal/mediascan"
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

// UpsertScanDirectory handles
// POST /api/config/directory-scanner/directories. scanner_slug in the body
// determines the entry: if a scan directory with that slug already exists
// it is replaced entirely, otherwise a new one is appended. Either way the
// full resulting document is fired as a system_config_update event.
//
// @Summary		Create or replace a scan directory
// @Description	Creates a new scan directory if scanner_slug doesn't already exist, or replaces every field of the existing entry otherwise. scanner_slug is required, and scan_type must be one of "movie", "tv" or "music_video".
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			request	body		appconfig.ScanDirectory	true	"Scan directory to create or replace"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body, missing scanner_slug, or unknown scan_type"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner/directories [post]
func (h *Handlers) UpsertScanDirectory(w http.ResponseWriter, r *http.Request) {
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
	// Reject an unscannable scan_type here rather than letting it fail later in
	// the scan listener, where nobody is waiting to see the error.
	if _, err := mediascan.ParseDirectoryType(entry.ScanType); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	if index := appConfig.DirectoryScanner.FindScanDirectoryIndex(entry.ScannerSlug); index == -1 {
		appConfig.DirectoryScanner.ScanDirectories = append(appConfig.DirectoryScanner.ScanDirectories, entry)
	} else {
		appConfig.DirectoryScanner.ScanDirectories[index] = entry
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

// ListSidecarTypes handles
// GET /api/config/directory-scanner/sidecar-types.
//
// @Summary		List sidecar types
// @Description	Reads the application config from MongoDB and returns the sidecar classification table. Each entry carries an id, which is what the other endpoints address it by, and an order, which is the sequence the scanner evaluates entries in. An order of 0 means the entry is disabled and never evaluated.
// @Tags			Config
// @Produce		json
// @Success		200	{array}	appconfig.SidecarTypeDefinition
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner/sidecar-types [get]
func (h *Handlers) ListSidecarTypes(w http.ResponseWriter, r *http.Request) {
	appConfig, err := h.AppConfigRepo.Get(r.Context())
	if err != nil {
		h.Logger.Error("failed to fetch app config", "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(appConfig.DirectoryScanner.SidecarTypes)
}

// GetSidecarType handles
// GET /api/config/directory-scanner/sidecar-types/{id}.
//
// @Summary		Fetch a single sidecar type
// @Description	Reads the application config from MongoDB and returns the sidecar type definition with the given id.
// @Tags			Config
// @Produce		json
// @Param			id	path		string	true	"sidecar type id"
// @Success		200	{object}	appconfig.SidecarTypeDefinition
// @Failure		404	{string}	string	"no sidecar type with that id"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner/sidecar-types/{id} [get]
func (h *Handlers) GetSidecarType(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	appConfig, err := h.AppConfigRepo.Get(r.Context())
	if err != nil {
		h.Logger.Error("failed to fetch app config", "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	index := appConfig.DirectoryScanner.FindSidecarTypeIndexByID(id)
	if index == -1 {
		http.Error(w, "no sidecar type with that id", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(appConfig.DirectoryScanner.SidecarTypes[index])
}

// UpsertSidecarType handles
// POST /api/config/directory-scanner/sidecar-types. The id in the body decides
// what happens: an empty one creates a new type, an existing one replaces that
// entry's definition.
//
// This endpoint never changes an entry's order. Order is a property of the table
// as a whole — the values have to stay unique, and moving one entry is really a
// statement about where it sits relative to the others — so it moves only
// through the ordering endpoint. A new type is therefore created disabled, and
// stays inert until it is given a slot, which means adding one can never quietly
// reclassify an existing library.
//
// @Summary		Create or replace a sidecar type
// @Description	Creates a new sidecar type when id is empty, minting one, or replaces the definition of an existing entry when id names one. type is required and must be unique, category must be one of the known categories, and every pattern must be a valid Go regular expression. This endpoint does not set order: a new type is created disabled (order 0), an existing type keeps the order it already had, and order is changed only through POST /api/config/directory-scanner/sidecar-types/order.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			request	body		appconfig.SidecarTypeDefinition	true	"Sidecar type to create or replace"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body, missing type, unknown category, an invalid pattern, or a non-zero order"
// @Failure		404		{string}	string	"no sidecar type with that id"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner/sidecar-types [post]
func (h *Handlers) UpsertSidecarType(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var entry appconfig.SidecarTypeDefinition
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if entry.Type == "" {
		http.Error(w, "type is required", http.StatusBadRequest)
		return
	}
	if _, err := mediascan.ParseSidecarCategory(entry.Category); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Rejected rather than ignored: silently dropping an order someone took the
	// trouble to send would leave them believing they had reordered the table.
	if entry.Order != 0 {
		http.Error(w, "order cannot be set here; use POST /api/config/directory-scanner/sidecar-types/order", http.StatusBadRequest)
		return
	}

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	if entry.ID == "" {
		// A new type is created disabled. It classifies nothing until the
		// ordering transaction gives it a place in the sequence.
		entry.ID = uuid.NewString()
		entry.Order = 0
		appConfig.DirectoryScanner.SidecarTypes = append(appConfig.DirectoryScanner.SidecarTypes, entry)
	} else {
		index := appConfig.DirectoryScanner.FindSidecarTypeIndexByID(entry.ID)
		if index == -1 {
			// An unknown id is a mistake worth surfacing, not an invitation to
			// create an entry under an id the caller chose.
			http.Error(w, "no sidecar type with that id", http.StatusNotFound)
			return
		}
		entry.Order = appConfig.DirectoryScanner.SidecarTypes[index].Order
		appConfig.DirectoryScanner.SidecarTypes[index] = entry
	}

	// Compile the resulting table, not just the submitted entry. A bad pattern
	// only becomes visible on compilation, and validating the whole table also
	// catches the duplicate a partial check would miss. Rejecting here means the
	// error reaches whoever is editing, rather than a scan log nobody is reading.
	if _, err := mediascan.NewSidecarRegistry(appConfig.DirectoryScanner.SidecarTypes); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
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

// DeleteSidecarType handles
// DELETE /api/config/directory-scanner/sidecar-types/{type}.
//
// @Summary		Delete a sidecar type
// @Description	Removes the sidecar type with the given id and fires system_config_update with the resulting document. Files that used to classify as this type will fall through to whatever the rest of the table says, or to "unknown". To stop a type classifying without losing its patterns, set its order to 0 through the ordering endpoint instead.
// @Tags			Config
// @Produce		json
// @Param			id	path		string	true	"sidecar type id"
// @Success		202	{object}	AcceptedResponse
// @Failure		404	{string}	string	"no sidecar type with that id"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner/sidecar-types/{id} [delete]
func (h *Handlers) DeleteSidecarType(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)
	id := r.PathValue("id")

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	index := appConfig.DirectoryScanner.FindSidecarTypeIndexByID(id)
	if index == -1 {
		http.Error(w, "no sidecar type with that id", http.StatusNotFound)
		return
	}
	sidecarTypes := appConfig.DirectoryScanner.SidecarTypes
	appConfig.DirectoryScanner.SidecarTypes = append(sidecarTypes[:index], sidecarTypes[index+1:]...)

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

// ReorderSidecarTypesRequest maps every sidecar type id to its evaluation order.
//
// An order of 0 disables an entry, so switching a type off and moving it are the
// same transaction — a disabled type keeps its patterns and starts classifying
// again the moment it is given a number.
//
// The map must name every stored id. Ordering is a property of the table as a
// whole: a partial update could create a duplicate against an order the caller
// never saw, and would make a reordering that shuffles several entries into a
// sequence of individually-invalid steps.
type ReorderSidecarTypesRequest map[string]int

// ReorderSidecarTypes handles
// POST /api/config/directory-scanner/sidecar-types/order.
//
// @Summary		Set the sidecar type evaluation order
// @Description	Assigns an evaluation order to every sidecar type in one transaction. The body maps sidecar type id to order; the scanner takes the first entry that accepts a file, so lower numbers win. An order of 0 disables an entry, which keeps it in the table but stops it classifying anything. Every stored id must appear, orders must be unique among enabled entries, and an unknown id is rejected.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			request	body		ReorderSidecarTypesRequest	true	"Map of sidecar type id to order"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body, an unknown or missing id, or a duplicate order"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner/sidecar-types/order [post]
func (h *Handlers) ReorderSidecarTypes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var requested ReorderSidecarTypesRequest
	if err := json.NewDecoder(r.Body).Decode(&requested); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}
	sidecarTypes := appConfig.DirectoryScanner.SidecarTypes

	// Every stored entry has to be accounted for, and nothing may be named that
	// does not exist. Both directions are checked before anything is applied, so
	// a rejected request leaves the stored order completely untouched.
	var missing []string
	for _, entry := range sidecarTypes {
		if _, present := requested[entry.ID]; !present {
			missing = append(missing, fmt.Sprintf("%s (%s)", entry.ID, entry.Type))
		}
	}
	if len(missing) > 0 {
		http.Error(w, "the order must name every sidecar type; missing: "+strings.Join(missing, ", "), http.StatusBadRequest)
		return
	}
	for id := range requested {
		if appConfig.DirectoryScanner.FindSidecarTypeIndexByID(id) == -1 {
			http.Error(w, "no sidecar type with id "+id, http.StatusBadRequest)
			return
		}
	}

	for i := range sidecarTypes {
		sidecarTypes[i].Order = requested[sidecarTypes[i].ID]
	}

	// The registry is the authority on what makes a coherent table, duplicate
	// orders included, so the result is run past it rather than duplicating the
	// rule here.
	if _, err := mediascan.NewSidecarRegistry(sidecarTypes); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	appConfig.DirectoryScanner.SidecarTypes = sidecarTypes

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

// ResetSidecarTypes handles
// POST /api/config/directory-scanner/sidecar-types/reset.
//
// This exists because the table is editable and therefore breakable: a user who
// deletes the wrong entries or saves a table that classifies nothing needs a way
// back that doesn't involve editing MongoDB by hand.
//
// @Summary		Restore the built-in sidecar types
// @Description	Replaces the entire sidecar classification table with the built-in defaults, discarding any customization, and fires system_config_update with the resulting document.
// @Tags			Config
// @Produce		json
// @Success		202	{object}	AcceptedResponse
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/directory-scanner/sidecar-types/reset [post]
func (h *Handlers) ResetSidecarTypes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	appConfig.DirectoryScanner.SidecarTypes = appconfig.DefaultSidecarTypes()

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
