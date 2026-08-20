package handlers

import (
	"encoding/json"
	"net/http"

	"Metarr/internal/server/workflow/validate"
	"Metarr/internal/shared/workflow"
)

// CatalogResponse is what the editor needs to render a palette and to
// pre-filter connections without asking the server on every drag.
type CatalogResponse struct {
	NodeTypes  []workflow.NodeType  `json:"node_types"`
	Transforms []workflow.Transform `json:"transforms"`
	// SchemaVersion is the graph format this server reads and writes, so the
	// editor can refuse to save a document it would downgrade.
	SchemaVersion int `json:"schema_version"`
}

// GetWorkflowCatalog handles GET /api/workflows/catalog.
//
// @Summary		Fetch the workflow node catalog
// @Description	Returns every installed node type plus the registry of explicit type conversions. The editor palette, server-side validation and the engine all read this same source.
// @Tags			Workflows
// @Produce		json
// @Success		200	{object}	CatalogResponse
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/workflows/catalog [get]
func (h *Handlers) GetWorkflowCatalog(w http.ResponseWriter, r *http.Request) {
	if h.WorkflowCatalog == nil {
		h.Logger.Error("workflow catalog is not loaded")
		http.Error(w, "workflow catalog is not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(CatalogResponse{
		NodeTypes:     h.WorkflowCatalog.All(),
		Transforms:    workflow.Transforms(),
		SchemaVersion: workflow.SchemaVersion,
	}); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// ValidateWorkflowRequest is a graph submitted for checking.
type ValidateWorkflowRequest struct {
	Graph workflow.Graph `json:"graph"`
}

// ValidateWorkflowResponse carries the diagnostics to paint on the canvas.
type ValidateWorkflowResponse struct {
	Diagnostics []validate.Diagnostic `json:"diagnostics"`
	// Runnable is false when any diagnostic is an error. Note that an
	// unrunnable graph is still perfectly saveable — people save half-built
	// flows, and refusing to store one would lose work.
	Runnable bool `json:"runnable"`
}

// ValidateWorkflow handles POST /api/workflows/validate.
//
// The editor calls this debounced as the graph changes. The client does its
// own cheap local checks during a drag — port kind, arity, type
// compatibility — but this is the authoritative one, because the whole-graph
// analyses (is this value guaranteed to exist here, do these branches
// converge) cannot be done from one endpoint's point of view.
//
// @Summary		Validate a workflow graph
// @Description	Statically checks a drawn graph and returns diagnostics. Errors block running, not saving.
// @Tags			Workflows
// @Accept			json
// @Produce		json
// @Param			request	body		ValidateWorkflowRequest	true	"Graph to validate"
// @Success		200		{object}	ValidateWorkflowResponse
// @Failure		400		{string}	string	"malformed graph"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/workflows/validate [post]
func (h *Handlers) ValidateWorkflow(w http.ResponseWriter, r *http.Request) {
	if h.WorkflowCatalog == nil {
		h.Logger.Error("workflow catalog is not loaded")
		http.Error(w, "workflow catalog is not available", http.StatusInternalServerError)
		return
	}

	var request ValidateWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "malformed graph", http.StatusBadRequest)
		return
	}

	result := validate.Graph(request.Graph, h.WorkflowCatalog)
	diagnostics := result.Diagnostics
	if diagnostics == nil {
		diagnostics = []validate.Diagnostic{}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ValidateWorkflowResponse{
		Diagnostics: diagnostics,
		Runnable:    result.Runnable(),
	}); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}
