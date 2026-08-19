package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// GetLoggingConfig handles GET /api/config/logging.
//
// @Summary		Fetch the logging configuration
// @Description	Returns the server's own log level and the informational pipeline fields shown on the System > Logging screen. Endpoint/sink/stream are informational only — the actual pipeline is Fluent Bit, configured independently of this document.
// @Tags			Config
// @Produce		json
// @Success		200	{object}	appconfig.LoggingConfig
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/logging [get]
func (h *Handlers) GetLoggingConfig(w http.ResponseWriter, r *http.Request) {
	appConfig, err := h.AppConfigRepo.Get(r.Context())
	if err != nil {
		h.Logger.Error("failed to fetch app config", "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(appConfig.Logging)
}

// UpsertLoggingConfig handles POST /api/config/logging. It replaces the whole
// logging section — a single upsert rather than a separate PUT, per the CRUD
// convention every config section here follows.
//
// @Summary		Update the logging configuration
// @Description	Replaces the logging section: the server's own log level, and the informational sink/endpoint/stream fields. Changing the informational fields does not reconfigure Fluent Bit — see the LoggingConfig doc comment.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			request	body		appconfig.LoggingConfig	true	"Logging configuration"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body or server_level"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/logging [post]
func (h *Handlers) UpsertLoggingConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var entry appconfig.LoggingConfig
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := validateLogLevel(entry.ServerLevel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	appConfig.Logging = entry

	if err := h.fireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		h.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue config update", http.StatusInternalServerError)
		return
	}

	h.writeAccepted(w, correlationID)
}

// setAgentLogLevelRequest is the body SetAgentLogLevel accepts.
type setAgentLogLevelRequest struct {
	LogLevel string `json:"log_level"`
}

// SetAgentLogLevel handles POST /api/config/agents/{slug}/log-level.
//
// It auto-creates a bare AgentConfig{Slug, LogLevel} entry when the named
// agent has connected but hasn't been configured with any libraries yet,
// rather than 404ing — bumping a not-yet-configured agent to debug is one of
// the more useful things to do with one, e.g. while working out why it isn't
// picking up a mapping. An agent that is already configured just gets its
// LogLevel field replaced; its mappings and display name are untouched.
//
// @Summary		Set one agent's log level
// @Description	Switches a single agent between info and debug. If the agent isn't configured with any libraries yet, this creates a minimal entry for it rather than requiring it be configured first.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			slug	path		string					true	"Agent slug"
// @Param			request	body		setAgentLogLevelRequest	true	"Log level"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body or log_level"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/agents/{slug}/log-level [post]
func (h *Handlers) SetAgentLogLevel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)
	slug := r.PathValue("slug")

	var body setAgentLogLevelRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := validateLogLevel(body.LogLevel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	if index := appConfig.FindAgentIndex(slug); index >= 0 {
		appConfig.Agents[index].LogLevel = body.LogLevel
	} else {
		appConfig.Agents = append(appConfig.Agents, appconfig.AgentConfig{
			Slug:     slug,
			LogLevel: body.LogLevel,
		})
	}

	if err := h.fireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		h.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue config update", http.StatusInternalServerError)
		return
	}

	h.writeAccepted(w, correlationID)
}

// GetLogTail handles GET /api/logging/tail. It returns the current contents
// of the server's log-tail buffer — the same data streamed continuously over
// the logging.tail WebSocket topic — so the Logging screen has something to
// show before its socket connects.
//
// @Summary		Fetch the recent log tail
// @Description	Returns the most recent log records seen on the centralized logging channel, from every process (server and every agent) that has published to it.
// @Tags			Config
// @Produce		json
// @Success		200	{array}	logging.Record
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/logging/tail [get]
func (h *Handlers) GetLogTail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.LogTail.Recent())
}

func validateLogLevel(level string) error {
	switch level {
	case appconfig.LogLevelInfo, appconfig.LogLevelDebug:
		return nil
	default:
		return fmt.Errorf("log_level must be %q or %q", appconfig.LogLevelInfo, appconfig.LogLevelDebug)
	}
}
