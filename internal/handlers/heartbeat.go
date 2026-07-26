package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"Metarr/internal/correlation"
	"Metarr/internal/eventbus"
)

// HeartbeatResponse is the payload returned by GET /api/heartbeat.
type HeartbeatResponse struct {
	Time          string `json:"time"`
	CorrelationID string `json:"correlation_id"`
}

// Heartbeat handles GET /api/heartbeat. It publishes a heartbeat request
// event onto the Redis Pub/Sub queue and blocks until the heartbeat
// listener replies with the current time and correlation ID, which is then
// returned to the client verbatim as the response body.
//
// @Summary		Blocking heartbeat check
// @Description	Publishes a heartbeat request on the Redis Pub/Sub queue and blocks until the heartbeat listener replies with the current time and the request's correlation ID.
// @Tags			Heartbeat
// @Produce		json
// @Success		200	{object}	HeartbeatResponse
// @Failure		504	{string}	string	"heartbeat timed out"
// @Router			/api/heartbeat [get]
func (h *Handlers) Heartbeat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	timeoutCtx, cancel := context.WithTimeout(ctx, h.HeartbeatTimeout)
	defer cancel()

	event := eventbus.Event{
		CorrelationID: correlationID,
		Name:          "heartbeat.request",
		Timestamp:     time.Now().UTC(),
	}

	reply, err := h.PubSub.Request(timeoutCtx, eventbus.HeartbeatRequestChannel, event)
	if err != nil {
		h.Logger.Error("heartbeat request failed", "correlation_id", correlationID, "error", err)
		if errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, "heartbeat timed out", http.StatusGatewayTimeout)
			return
		}
		http.Error(w, "heartbeat failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(reply.Payload)
}
