package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/eventbus"
)

// ListAgents handles GET /api/config/agents. It returns every agent the server
// knows about: the ones configured here, the ones currently connected, and the
// union of the two.
//
// An agent that has connected but has not been configured appears with
// configured=false. That is the state every new agent starts in and the one the
// UI exists to surface — nothing starts reading a remote filesystem just
// because a process appeared on the network.
//
// @Summary		List agents
// @Description	Returns configured agents merged with the agents currently connected to Redis, including live host telemetry for those that are online.
// @Tags			Config
// @Produce		json
// @Success		200	{array}		agentregistry.AgentView
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/agents [get]
func (h *Handlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	agents, err := h.Agents.List(ctx, appConfig)
	if err != nil {
		h.Logger.Error("failed to list agents", "error", err)
		http.Error(w, "failed to list agents", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

// UpsertAgent handles POST /api/config/agents. The slug in the body determines
// the entry: an existing agent is replaced entirely, a new one appended.
//
// @Summary		Create or replace an agent
// @Description	Creates a new agent configuration if the slug does not exist, or replaces every field of the existing entry otherwise. Each mapping ties one scan directory to the path that agent knows it by; a scan directory the agent cannot see is simply left out.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			request	body		appconfig.AgentConfig	true	"Agent to create or replace"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body, bad slug, or an unusable mapping"
// @Failure		409		{string}	string	"a scan directory is already mapped to a different agent"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/agents [post]
func (h *Handlers) UpsertAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var entry appconfig.AgentConfig
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// The slug is embedded directly in Redis keys and stream names, so it is
	// validated with the same rule the agent applies to its own local config.
	if err := agentproto.ValidateSlug(entry.Slug); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	if status, err := validateMappings(appConfig, entry); err != nil {
		http.Error(w, err.Error(), status)
		return
	}

	if index := appConfig.FindAgentIndex(entry.Slug); index == -1 {
		appConfig.Agents = append(appConfig.Agents, entry)
	} else {
		appConfig.Agents[index] = entry
	}

	if err := h.fireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		h.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue config update", http.StatusInternalServerError)
		return
	}

	h.writeAccepted(w, correlationID)
}

// DeleteAgent handles DELETE /api/config/agents/{slug}.
//
// @Summary		Delete an agent
// @Description	Removes an agent's configuration and the copy it reads from Redis, so a machine that reconnects later does not resume scanning a library it is no longer meant to touch. The agent process itself is unaffected and will reappear as unconfigured while it keeps running.
// @Tags			Config
// @Produce		json
// @Param			slug	path		string	true	"Agent slug"
// @Success		202		{object}	AcceptedResponse
// @Failure		404		{string}	string	"no agent with that slug"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/agents/{slug} [delete]
func (h *Handlers) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)
	slug := r.PathValue("slug")

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	index := appConfig.FindAgentIndex(slug)
	if index == -1 {
		http.Error(w, "no agent with that slug", http.StatusNotFound)
		return
	}

	appConfig.Agents = append(appConfig.Agents[:index], appConfig.Agents[index+1:]...)

	if err := h.fireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		h.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue config update", http.StatusInternalServerError)
		return
	}

	// Drop the published projection immediately rather than waiting for the
	// config listener: a deleted agent should stop being able to read its
	// mapping now, not once the event has been processed.
	if err := h.Agents.Forget(ctx, slug); err != nil {
		h.Logger.Warn("could not remove the agent's published configuration", "agent", slug, "error", err)
	}

	h.writeAccepted(w, correlationID)
}

// validateMappings rejects an agent whose mappings could not work, returning
// the HTTP status to answer with.
func validateMappings(config *appconfig.Config, entry appconfig.AgentConfig) (int, error) {
	seen := map[string]bool{}

	for _, mapping := range entry.Mappings {
		if config.DirectoryScanner.FindScanDirectoryIndex(mapping.ScannerSlug) < 0 {
			return http.StatusBadRequest,
				fmt.Errorf("no scan directory with slug %q", mapping.ScannerSlug)
		}
		if seen[mapping.ScannerSlug] {
			return http.StatusBadRequest,
				fmt.Errorf("scan directory %q is mapped twice", mapping.ScannerSlug)
		}
		seen[mapping.ScannerSlug] = true

		// Two agents scanning one library would each overwrite the other's
		// records with its own view of the same files, so a scan directory
		// belongs to exactly one agent.
		if owner, mapped := config.AgentForScanner(mapping.ScannerSlug); mapped && owner.Slug != entry.Slug {
			return http.StatusConflict,
				fmt.Errorf("scan directory %q is already mapped to agent %q", mapping.ScannerSlug, owner.Slug)
		}
	}

	return 0, nil
}

// writeAccepted sends the queued-not-stored response every config mutation
// returns.
func (h *Handlers) writeAccepted(w http.ResponseWriter, correlationID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(AcceptedResponse{
		Status:        "queued",
		Event:         eventbus.SystemConfigUpdateEventName,
		CorrelationID: correlationID,
	})
}
