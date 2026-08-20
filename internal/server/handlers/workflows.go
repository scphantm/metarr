package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"

	"Metarr/internal/server/mongostore"
	"Metarr/internal/server/mongostore/versioned"
)

const (
	defaultWorkflowLimit = 20
	maxWorkflowLimit     = 100
)

// WorkflowListResponse is the cursor-paginated response for ListWorkflows.
type WorkflowListResponse struct {
	Workflows  []mongostore.Workflow `json:"workflows"`
	NextCursor string                `json:"next_cursor,omitempty"`
	HasMore    bool                  `json:"has_more"`
}

// ListWorkflows handles GET /api/workflows.
//
// @Summary		List workflows
// @Description	Returns the latest version of every saved workflow, newest first, paginated by an opaque cursor.
// @Tags			Workflows
// @Produce		json
// @Param			limit	query		int		false	"Maximum records to return (default 20, max 100)"
// @Param			cursor	query		string	false	"Opaque cursor from a previous page's next_cursor"
// @Success		200		{object}	WorkflowListResponse
// @Failure		400		{string}	string	"invalid limit or cursor"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/workflows [get]
func (h *Handlers) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	filter := versioned.LatestFilter{Limit: defaultWorkflowLimit}

	if rawLimit := query.Get("limit"); rawLimit != "" {
		limit, err := strconv.ParseInt(rawLimit, 10, 64)
		if err != nil || limit < 1 {
			http.Error(w, "limit must be a positive integer", http.StatusBadRequest)
			return
		}
		if limit > maxWorkflowLimit {
			limit = maxWorkflowLimit
		}
		filter.Limit = limit
	}

	if rawCursor := query.Get("cursor"); rawCursor != "" {
		cursor, err := bson.ObjectIDFromHex(rawCursor)
		if err != nil {
			http.Error(w, "malformed cursor", http.StatusBadRequest)
			return
		}
		filter.Cursor = cursor
	}

	workflows, nextCursor, hasMore, err := h.WorkflowRepo.ListLatest(r.Context(), filter)
	if err != nil {
		h.Logger.Error("failed to list workflows", "error", err)
		http.Error(w, "failed to list workflows", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(WorkflowListResponse{
		Workflows:  workflows,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// GetWorkflow handles GET /api/workflows/{id}.
//
// @Summary		Fetch a workflow
// @Description	Returns the latest version of one workflow.
// @Tags			Workflows
// @Produce		json
// @Param			id	path		string	true	"Workflow id"
// @Success		200	{object}	mongostore.Workflow
// @Failure		400	{string}	string	"malformed id"
// @Failure		404	{string}	string	"no workflow with that id"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/workflows/{id} [get]
func (h *Handlers) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := h.parseRecordID(w, r.PathValue("id"))
	if !ok {
		return
	}

	workflow, err := h.WorkflowRepo.GetLatest(r.Context(), workflowID)
	if err != nil {
		h.writeWorkflowLookupError(w, workflowID, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// ListWorkflowVersions handles GET /api/workflows/{id}/versions.
//
// @Summary		List a workflow's versions
// @Description	Returns every saved version of one workflow, newest first, for the version-history strip.
// @Tags			Workflows
// @Produce		json
// @Param			id	path		string	true	"Workflow id"
// @Success		200	{array}		mongostore.Workflow
// @Failure		400	{string}	string	"malformed id"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/workflows/{id}/versions [get]
func (h *Handlers) ListWorkflowVersions(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := h.parseRecordID(w, r.PathValue("id"))
	if !ok {
		return
	}

	versions, err := h.WorkflowRepo.ListVersions(r.Context(), workflowID)
	if err != nil {
		h.Logger.Error("failed to list workflow versions", "workflow_id", workflowID.Hex(), "error", err)
		http.Error(w, "failed to list workflow versions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(versions); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// GetWorkflowVersion handles GET /api/workflows/{id}/versions/{version}.
//
// @Summary		Fetch one workflow version
// @Description	Returns one specific past version of a workflow, exactly as it was saved.
// @Tags			Workflows
// @Produce		json
// @Param			id		path		string	true	"Workflow id"
// @Param			version	path		int		true	"Version number"
// @Success		200		{object}	mongostore.Workflow
// @Failure		400		{string}	string	"malformed id or version"
// @Failure		404		{string}	string	"no such workflow or version"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/workflows/{id}/versions/{version} [get]
func (h *Handlers) GetWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := h.parseRecordID(w, r.PathValue("id"))
	if !ok {
		return
	}

	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || version < 1 {
		http.Error(w, "version must be a positive integer", http.StatusBadRequest)
		return
	}

	workflow, err := h.WorkflowRepo.GetVersion(r.Context(), workflowID, version)
	if err != nil {
		h.writeWorkflowLookupError(w, workflowID, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// UpsertWorkflow handles POST /api/workflows.
//
// Unlike most config-mutation endpoints, this writes straight to Mongo and
// responds synchronously: workflows are a server-only, single-collection
// concern with no agent fan-out, so there is no event to fire. A missing
// document_id creates a new workflow (version 1); a present one appends a new
// version — nothing is ever overwritten in place. Either way the full
// persisted document is returned so the caller has the (possibly new) id and
// version number without a follow-up request.
//
// @Summary		Save a workflow
// @Description	Creates a new workflow, or appends a new version to an existing one if document_id is set. Every save is a new version — nothing is overwritten in place.
// @Tags			Workflows
// @Accept			json
// @Produce		json
// @Param			workflow	body		mongostore.Workflow	true	"Workflow to save"
// @Success		201			{object}	mongostore.Workflow
// @Failure		400			{string}	string	"malformed body, or missing name/description/tags"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/workflows [post]
func (h *Handlers) UpsertWorkflow(w http.ResponseWriter, r *http.Request) {
	var entry mongostore.Workflow
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "malformed workflow", http.StatusBadRequest)
		return
	}

	if entry.Name == "" || entry.Description == "" || len(entry.Tags) == 0 {
		http.Error(w, "name, description and at least one tag are required", http.StatusBadRequest)
		return
	}

	saved, err := h.WorkflowRepo.Save(r.Context(), entry.DocumentID, entry)
	if err != nil {
		h.Logger.Error("failed to save workflow", "error", err)
		http.Error(w, "failed to save workflow", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(saved); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

func (h *Handlers) writeWorkflowLookupError(w http.ResponseWriter, workflowID bson.ObjectID, err error) {
	if errors.Is(err, versioned.ErrNotFound) {
		http.Error(w, "no workflow with that id", http.StatusNotFound)
		return
	}
	h.Logger.Error("failed to fetch workflow", "workflow_id", workflowID.Hex(), "error", err)
	http.Error(w, "failed to fetch workflow", http.StatusInternalServerError)
}
