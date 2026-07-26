package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"Metarr/internal/correlation"
	"Metarr/internal/eventbus"
)

type taskRequest struct {
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
// @Param			request	body		taskRequest	true	"Command to run"
// @Success		202		{object}	acceptedResponse
// @Failure		400		{string}	string	"invalid request body or unsupported command"
// @Router			/api/tasks/sonarr_cache_data [post]
func (h *Handlers) SonarrCacheData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var req taskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Command != "run" {
		http.Error(w, `unsupported command, expected "run"`, http.StatusBadRequest)
		return
	}

	evt := eventbus.Event{
		CorrelationID: correlationID,
		Name:          eventbus.SonarrCacheDataEventName,
		Timestamp:     time.Now().UTC(),
	}

	if err := h.Streams.Fire(ctx, eventbus.SonarrCacheDataStream, evt); err != nil {
		h.Logger.Error("failed to fire sonarr_cache_data event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue task", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(acceptedResponse{
		Status:        "accepted",
		Event:         eventbus.SonarrCacheDataEventName,
		CorrelationID: correlationID,
	})
}
