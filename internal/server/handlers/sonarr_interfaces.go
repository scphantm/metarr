package handlers

import (
	"encoding/json"
	"net/http"

	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/eventbus"
)

// ListSonarrInterfaces handles GET /api/config/interfaces/sonarr. It reads
// the application config straight from MongoDB and returns every
// configured Sonarr instance.
//
// @Summary		List Sonarr interface instances
// @Description	Reads the application config from MongoDB and returns every configured Sonarr instance.
// @Tags			Config
// @Produce		json
// @Success		200	{array}	appconfig.SonarrInstance
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/interfaces/sonarr [get]
func (h *Handlers) ListSonarrInterfaces(w http.ResponseWriter, r *http.Request) {
	appConfig, err := h.AppConfigRepo.Get(r.Context())
	if err != nil {
		h.Logger.Error("failed to fetch app config", "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(appConfig.Interfaces.Sonarr); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// GetSonarrInterface handles GET /api/config/interfaces/sonarr/{slug}.
//
// @Summary		Fetch a single Sonarr interface instance
// @Description	Reads the application config from MongoDB and returns the Sonarr instance with the given instance_slug.
// @Tags			Config
// @Produce		json
// @Param			slug	path		string	true	"instance_slug"
// @Success		200		{object}	appconfig.SonarrInstance
// @Failure		404		{string}	string	"no Sonarr instance with that slug"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/interfaces/sonarr/{slug} [get]
func (h *Handlers) GetSonarrInterface(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	appConfig, err := h.AppConfigRepo.Get(r.Context())
	if err != nil {
		h.Logger.Error("failed to fetch app config", "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	index := appConfig.Interfaces.FindSonarrIndex(slug)
	if index == -1 {
		http.Error(w, "no Sonarr instance with that slug", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(appConfig.Interfaces.Sonarr[index]); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// UpsertSonarrInterface handles POST /api/config/interfaces/sonarr.
// instance_slug in the body determines the entry: if a Sonarr instance with
// that slug already exists it is replaced entirely; otherwise a new one is
// appended, provided the slug isn't already claimed by a different
// interface type. instance_slug is unique across all interface types, not
// just within Sonarr — see appconfig.InterfacesConfig.AllInstanceSlugs —
// so that invariant still has to be enforced on the insert path even though
// this is now an upsert.
//
// @Summary		Create or replace a Sonarr interface instance
// @Description	Creates a new Sonarr instance if instance_slug doesn't already exist, or replaces every field of the existing entry otherwise. instance_slug is required and must be unique across every interface type.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			request	body		appconfig.SonarrInstance	true	"Sonarr instance to create or replace"
// @Success		202		{object}	AcceptedResponse
// @Failure		400		{string}	string	"invalid request body or missing instance_slug"
// @Failure		409		{string}	string	"instance_slug already in use by a different interface type"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/interfaces/sonarr [post]
func (h *Handlers) UpsertSonarrInterface(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)

	var instance appconfig.SonarrInstance
	if err := json.NewDecoder(r.Body).Decode(&instance); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if instance.InstanceSlug == "" {
		http.Error(w, "instance_slug is required", http.StatusBadRequest)
		return
	}

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	index := appConfig.Interfaces.FindSonarrIndex(instance.InstanceSlug)
	if index == -1 {
		for _, slug := range appConfig.Interfaces.AllInstanceSlugs() {
			if slug == instance.InstanceSlug {
				http.Error(w, "instance_slug already in use by a different interface type", http.StatusConflict)
				return
			}
		}
		appConfig.Interfaces.Sonarr = append(appConfig.Interfaces.Sonarr, instance)
	} else {
		appConfig.Interfaces.Sonarr[index] = instance
	}

	if err := h.fireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		h.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue config update", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(AcceptedResponse{
		Status:        "accepted",
		Event:         eventbus.SystemConfigUpdateEventName,
		CorrelationID: correlationID,
	}); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// DeleteSonarrInterface handles DELETE /api/config/interfaces/sonarr/{slug}.
//
// @Summary		Delete a Sonarr interface instance
// @Description	Removes the Sonarr instance with the given instance_slug and fires system_config_update with the resulting document.
// @Tags			Config
// @Produce		json
// @Param			slug	path		string	true	"instance_slug"
// @Success		202		{object}	AcceptedResponse
// @Failure		404		{string}	string	"no Sonarr instance with that slug"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/interfaces/sonarr/{slug} [delete]
func (h *Handlers) DeleteSonarrInterface(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)
	slug := r.PathValue("slug")

	appConfig, err := h.AppConfigRepo.Get(ctx)
	if err != nil {
		h.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to fetch config", http.StatusInternalServerError)
		return
	}

	index := appConfig.Interfaces.FindSonarrIndex(slug)
	if index == -1 {
		http.Error(w, "no Sonarr instance with that slug", http.StatusNotFound)
		return
	}
	appConfig.Interfaces.Sonarr = append(appConfig.Interfaces.Sonarr[:index], appConfig.Interfaces.Sonarr[index+1:]...)

	if err := h.fireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		h.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		http.Error(w, "failed to queue config update", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(AcceptedResponse{
		Status:        "accepted",
		Event:         eventbus.SystemConfigUpdateEventName,
		CorrelationID: correlationID,
	}); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}
