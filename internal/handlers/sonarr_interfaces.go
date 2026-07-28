package handlers

import (
	"encoding/json"
	"net/http"

	"Metarr/internal/appconfig"
	"Metarr/internal/correlation"
	"Metarr/internal/eventbus"
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
	json.NewEncoder(w).Encode(appConfig.Interfaces.Sonarr)
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
	json.NewEncoder(w).Encode(appConfig.Interfaces.Sonarr[index])
}

// CreateSonarrInterface handles POST /api/config/interfaces/sonarr. It
// validates instance_slug is present and unique across every interface
// type, appends the new instance, and fires system_config_update with the
// full resulting document — the SystemConfigUpdate listener persists it and
// refreshes the in-memory config singleton asynchronously.
//
// @Summary		Create a Sonarr interface instance
// @Description	Adds a new Sonarr instance. instance_slug is required and must be unique across every interface type.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			request	body		appconfig.SonarrInstance	true	"New Sonarr instance"
// @Success		202		{object}	acceptedResponse
// @Failure		400		{string}	string	"invalid request body or missing instance_slug"
// @Failure		409		{string}	string	"instance_slug already in use"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/interfaces/sonarr [post]
func (h *Handlers) CreateSonarrInterface(w http.ResponseWriter, r *http.Request) {
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

	for _, slug := range appConfig.Interfaces.AllInstanceSlugs() {
		if slug == instance.InstanceSlug {
			http.Error(w, "instance_slug already in use", http.StatusConflict)
			return
		}
	}

	appConfig.Interfaces.Sonarr = append(appConfig.Interfaces.Sonarr, instance)

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

// UpdateSonarrInterface handles PUT /api/config/interfaces/sonarr/{slug}.
// instance_slug can never be changed once set: if the body sets a
// different, non-empty instance_slug than the URL, the request is
// rejected. Every other field is replaced with what the body supplies.
//
// @Summary		Update a Sonarr interface instance
// @Description	Replaces every field of the Sonarr instance at the given instance_slug except instance_slug itself, which cannot be changed once set.
// @Tags			Config
// @Accept			json
// @Produce		json
// @Param			slug	path		string						true	"instance_slug"
// @Param			request	body		appconfig.SonarrInstance	true	"Updated Sonarr instance"
// @Success		202		{object}	acceptedResponse
// @Failure		400		{string}	string	"invalid request body, or attempted to change instance_slug"
// @Failure		404		{string}	string	"no Sonarr instance with that slug"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/config/interfaces/sonarr/{slug} [put]
func (h *Handlers) UpdateSonarrInterface(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := correlation.FromContext(ctx)
	slug := r.PathValue("slug")

	var instance appconfig.SonarrInstance
	if err := json.NewDecoder(r.Body).Decode(&instance); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if instance.InstanceSlug != "" && instance.InstanceSlug != slug {
		http.Error(w, "instance_slug cannot be changed", http.StatusBadRequest)
		return
	}
	instance.InstanceSlug = slug

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
	appConfig.Interfaces.Sonarr[index] = instance

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

// DeleteSonarrInterface handles DELETE /api/config/interfaces/sonarr/{slug}.
//
// @Summary		Delete a Sonarr interface instance
// @Description	Removes the Sonarr instance with the given instance_slug and fires system_config_update with the resulting document.
// @Tags			Config
// @Produce		json
// @Param			slug	path		string	true	"instance_slug"
// @Success		202		{object}	acceptedResponse
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
	json.NewEncoder(w).Encode(acceptedResponse{
		Status:        "accepted",
		Event:         eventbus.SystemConfigUpdateEventName,
		CorrelationID: correlationID,
	})
}
