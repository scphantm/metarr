package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"Metarr/internal/appconfig"
	"Metarr/internal/correlation"
	"Metarr/internal/eventbus"
)

// GetConfig handles GET /api/config. It reads the application config
// straight from MongoDB (the source of truth) and returns it to the
// client.
//
// @Summary		Fetch the application config
// @Description	Reads the singleton application config document from MongoDB.
// @Tags			Config
// @Produce		json
// @Success		200	{object}	appconfig.Config
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config [get]
func (h *Handlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	appConfig, err := h.AppConfigRepo.Get(r.Context())
	if err != nil {
		h.Logger.Error("failed to fetch app config", "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	// Redact the password hash/salt before this ever reaches a client.
	// They round-trip through JSON internally (system_config_update event
	// payloads), so the model can't hide them with a hard `json:"-"`.
	appConfig.Admin.PasswordSalt = ""
	appConfig.Admin.PasswordHash = ""

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(appConfig)
}

// UpdateConfig handles PUT /api/config. It fires the system_config_update
// event with the updated document as its payload and returns to the client
// as soon as the event is fired — the SystemConfigUpdate listener persists
// the change to MongoDB and refreshes the in-memory config singleton
// asynchronously.
//
// @Summary		Update the application config
// @Description	Fires a system_config_update event with the updated document as its payload and returns as soon as the event has been queued. The SystemConfigUpdate listener persists the change to MongoDB and then refreshes the in-memory config singleton.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			request	body		appconfig.Config	true	"Updated application config"
// @Success		202		{object}	acceptedResponse
// @Failure		400		{string}	string	"invalid request body"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config [put]
func (h *Handlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var updatedConfig appconfig.Config
	if err := json.NewDecoder(r.Body).Decode(&updatedConfig); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.fireConfigUpdate(ctx, correlationID, updatedConfig); err != nil {
		h.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue config update", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(acceptedResponse{
		Status:        "accepted",
		Event:         eventbus.SystemConfigUpdateEventName,
		CorrelationID: correlationID,
	})
}

// fireConfigUpdate marshals config and fires it as a system_config_update
// event. Used for both whole-document updates (UpdateConfig) and
// per-interface-instance CRUD, which read-modify-fire the same full
// document so the existing SystemConfigUpdate listener can persist it and
// refresh the in-memory config singleton without any changes of its own.
func (h *Handlers) fireConfigUpdate(ctx context.Context, correlationID string, config appconfig.Config) error {
	payload, err := json.Marshal(config)
	if err != nil {
		return err
	}

	event := eventbus.Event{
		CorrelationID: correlationID,
		Name:          eventbus.SystemConfigUpdateEventName,
		Payload:       payload,
		Timestamp:     time.Now().UTC(),
	}

	return h.Streams.Fire(ctx, eventbus.SystemConfigUpdateStream, event)
}
