package handlers

import (
	"encoding/json"
	"net/http"

	"Metarr/internal/appconfig"
	"Metarr/internal/auth"
	"Metarr/internal/correlation"
	"Metarr/internal/passwordhash"
	"Metarr/internal/session"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	APIKey           string `json:"api_key"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
}

type logoutResponse struct {
	Status string `json:"status"`
}

// Login handles POST /api/auth/login. It compares the submitted
// username/password against the stored admin credentials and, on a match,
// issues a new session API key (valid for session.TTL) carrying admin
// rights.
//
// @Summary		Log in as the admin user
// @Description	Compares the submitted username/password against the stored admin credentials and issues a session API key carrying admin rights.
// @Tags			Auth
// @Accept			json
// @Produce		json
// @Param			request	body		loginRequest	true	"Login credentials"
// @Success		200		{object}	loginResponse
// @Failure		400		{string}	string	"invalid request body"
// @Failure		401		{string}	string	"invalid username or password"
// @Router			/api/auth/login [post]
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	admin := appconfig.Get().Admin
	if admin.Username == "" || req.Username == "" || req.Username != admin.Username ||
		!passwordhash.Verify(req.Password, admin.PasswordSalt, admin.PasswordHash) {
		h.Logger.Warn("login failed", "correlation_id", correlationID, "username", req.Username)
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}

	apiKey, err := h.Sessions.Create(ctx)
	if err != nil {
		h.Logger.Error("failed to create session", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(loginResponse{
		APIKey:           apiKey,
		ExpiresInSeconds: int(session.TTL.Seconds()),
	})
}

// Logout handles POST /api/auth/logout. It revokes the session API key
// that authenticated the current request.
//
// @Summary		Log out
// @Description	Revokes the session API key that authenticated this request.
// @Tags			Auth
// @Produce		json
// @Success		200	{object}	logoutResponse
// @Router			/api/auth/logout [post]
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)
	apiKey := auth.APIKeyFromContext(ctx)

	if err := h.Sessions.Delete(ctx, apiKey); err != nil {
		h.Logger.Error("failed to delete session", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to log out", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logoutResponse{Status: "logged_out"})
}
