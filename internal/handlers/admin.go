package handlers

import (
	"encoding/json"
	"net/http"

	"Metarr/internal/correlation"
	"Metarr/internal/eventbus"
	"Metarr/internal/passwordhash"
)

// updateAdminRequest is a partial update: only the fields present are
// changed. An explicitly-empty string for a provided field is rejected
// rather than silently clearing it.
type updateAdminRequest struct {
	Username *string `json:"username,omitempty"`
	Email    *string `json:"email,omitempty"`
	Password *string `json:"password,omitempty"`
}

// UpdateAdmin handles PUT /api/config/admin. It fires the
// system_config_update event with the updated document as its payload and
// returns to the client as soon as the event is fired — the
// SystemConfigUpdate listener persists the change to MongoDB and refreshes
// the in-memory config singleton asynchronously, exactly like UpdateConfig.
//
// @Summary		Update the admin user's credentials
// @Description	Updates any subset of the admin user's username, email, and password. A provided field cannot be empty. If password is set, it is re-hashed with a fresh salt.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			request	body		updateAdminRequest	true	"Fields to update"
// @Success		202		{object}	acceptedResponse
// @Failure		400		{string}	string	"invalid request body, or a provided field was empty"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/admin [put]
func (h *Handlers) UpdateAdmin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var req updateAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Username != nil && *req.Username == "" {
		http.Error(w, "username cannot be empty", http.StatusBadRequest)
		return
	}
	if req.Email != nil && *req.Email == "" {
		http.Error(w, "email cannot be empty", http.StatusBadRequest)
		return
	}
	if req.Password != nil && *req.Password == "" {
		http.Error(w, "password cannot be empty", http.StatusBadRequest)
		return
	}

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	if req.Username != nil {
		appConfig.Admin.Username = *req.Username
	}
	if req.Email != nil {
		appConfig.Admin.Email = *req.Email
	}
	if req.Password != nil {
		salt, hash, err := passwordhash.Hash(*req.Password)
		if err != nil {
			h.Logger.Error("failed to hash password", "correlation_id", correlationID, "error", err)
			http.Error(w, "failed to update admin credentials", http.StatusInternalServerError)
			return
		}
		appConfig.Admin.PasswordSalt = salt
		appConfig.Admin.PasswordHash = hash
	}

	if err := h.fireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
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
