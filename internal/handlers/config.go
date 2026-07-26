package handlers

import (
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
// @Router			/api/config [get]
func (h *Handlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	appConfig, err := h.AppConfigRepo.Get(r.Context())
	if err != nil {
		h.Logger.Error("failed to fetch app config", "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

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
// @Router			/api/config [put]
func (h *Handlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var updatedConfig appconfig.Config
	if err := json.NewDecoder(r.Body).Decode(&updatedConfig); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	payload, err := json.Marshal(updatedConfig)
	if err != nil {
		h.Logger.Error("failed to encode config payload", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to encode config", http.StatusInternalServerError)
		return
	}

	event := eventbus.Event{
		CorrelationID: correlationID,
		Name:          eventbus.SystemConfigUpdateEventName,
		Payload:       payload,
		Timestamp:     time.Now().UTC(),
	}

	if err := h.Streams.Fire(ctx, eventbus.SystemConfigUpdateStream, event); err != nil {
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
