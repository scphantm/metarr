package handlers

import (
	"encoding/json"
	"net/http"
)

// GetRedisStats handles GET /api/stats/redis. It returns a point-in-time
// snapshot of the Redis instance behind the event system.
//
// The same snapshot streams continuously over the stats.redis WebSocket
// topic. This endpoint remains because a dashboard needs something to paint
// before its socket has connected, and something to fall back to when the
// socket is down.
//
// @Summary		Fetch Redis statistics
// @Description	Returns the depth and consumer-group state of every event stream, the subscriber count of every Pub/Sub channel, and server-wide counters. The same data streams over the stats.redis WebSocket topic.
// @Tags			Stats
// @Produce		json
// @Success		200	{object}	redisstats.Snapshot
// @Failure		500	{string}	string	"failed to collect redis statistics"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/stats/redis [get]
func (h *Handlers) GetRedisStats(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.Stats.Collect(r.Context())
	if err != nil {
		h.Logger.Error("failed to collect redis statistics", "error", err)
		http.Error(w, "failed to collect redis statistics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot)
}
