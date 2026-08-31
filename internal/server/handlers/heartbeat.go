package handlers

import (
	"context"
	"errors"
	"net/http"

	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/eventbus"
)

// HeartbeatResponse is the payload returned by GET /api/heartbeat.
type HeartbeatResponse struct {
	Time          string `json:"time"`
	CorrelationID string `json:"correlation_id"`
	Version       string `json:"version"`
}

// Heartbeat handles GET /api/heartbeat. It is a Redis round-trip health
// check: it publishes a request on a Pub/Sub channel and blocks on the
// correlation-scoped reply channel until the heartbeat listener — also in
// metarr-server — answers with the current time and correlation ID, which is
// returned to the client verbatim. It exercises the same request/reply path
// (eventbus.PubSubBus.Request) the NFO read uses, so a green heartbeat means
// the server can publish, subscribe, and round-trip through Redis. It does
// not reach an agent.
//
// @Summary		Redis round-trip health check
// @Description	Publishes a request on a Redis Pub/Sub channel and blocks until the in-process heartbeat listener replies with the current time and the request's correlation ID. Confirms the server's Redis request/reply path; it does not reach any agent.
// @Tags			Heartbeat
// @Produce		json
// @Success		200	{object}	HeartbeatResponse
// @Failure		429	{string}	string	"too many requests"
// @Failure		504	{string}	string	"heartbeat timed out"
// @Router			/api/heartbeat [get]
func (h *Handlers) Heartbeat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	timeoutCtx, cancel := context.WithTimeout(ctx, h.HeartbeatTimeout)
	defer cancel()

	event := eventbus.NewEvent(eventbus.SourceServer, "heartbeat.request", correlationID, nil)

	reply, err := h.PubSub.Request(timeoutCtx, eventbus.HeartbeatRequestChannel, event)
	if err != nil {
		h.Logger.Error("heartbeat request failed", "correlation_id", correlationID, "error", err)
		if errors.Is(err, eventbus.ErrNoResponder) {
			http.Error(w, "heartbeat timed out", http.StatusGatewayTimeout)
			return
		}
		http.Error(w, "heartbeat failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(reply.Payload); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}
