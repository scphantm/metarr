package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// GetChatbotConfig handles GET /api/config/chatbot.
//
// @Summary		Fetch the chatbot configuration
// @Description	Returns settings for all four providers (only Provider's is active) — API keys are returned in plaintext, unredacted, matching how Sonarr's API key is already handled.
// @Tags			Config
// @Produce		json
// @Success		200	{object}	appconfig.ChatbotConfig
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/chatbot [get]
func (h *Handlers) GetChatbotConfig(w http.ResponseWriter, r *http.Request) {
	appConfig, err := h.AppConfigRepo.Get(r.Context())
	if err != nil {
		h.Logger.Error("failed to fetch app config", "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(appConfig.Chatbot); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// UpsertChatbotConfig handles POST /api/config/chatbot. It replaces the
// whole chatbot section — a single upsert rather than a separate PUT, per
// the CRUD convention every config section here follows.
//
// @Summary		Update the chatbot configuration
// @Description	Replaces the chatbot section. Settings for all four providers are stored even though only Provider's are active, so switching providers back and forth never drops what was entered.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			request	body		appconfig.ChatbotConfig	true	"Chatbot configuration"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body or provider"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/chatbot [post]
func (h *Handlers) UpsertChatbotConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var entry appconfig.ChatbotConfig
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := validateChatbotProvider(entry.Provider); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	appConfig.Chatbot = entry

	if err := h.fireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		h.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue config update", http.StatusInternalServerError)
		return
	}

	h.writeAccepted(w, correlationID)
}

func validateChatbotProvider(provider string) error {
	switch provider {
	case appconfig.ChatbotProviderClaude, appconfig.ChatbotProviderOpenAI, appconfig.ChatbotProviderGemini, appconfig.ChatbotProviderMCP:
		return nil
	default:
		return fmt.Errorf("provider must be one of %q, %q, %q, %q",
			appconfig.ChatbotProviderClaude, appconfig.ChatbotProviderOpenAI, appconfig.ChatbotProviderGemini, appconfig.ChatbotProviderMCP)
	}
}
