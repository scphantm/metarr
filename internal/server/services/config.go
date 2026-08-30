package services

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/server/passwordhash"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// ConfigServer implements metarrv1connect.ConfigServiceHandler. Every write
// goes through AppConfigStore.Mutate — see internal/server/appconfigstore
// and ADR 0001/0002 for why.
type ConfigServer struct {
	*handlers.Handlers
}

// ConfigAuthPolicies is this service's method-name -> policy map. Mirrors
// every config route in router.go being GroupConfig.
var ConfigAuthPolicies = map[string]httpserver.RPCPolicy{
	"Get":          {Group: auth.GroupConfig, ReadOnly: true},
	"UpdateAdmin":  {Group: auth.GroupConfig},
	"UpsertApiKey": {Group: auth.GroupConfig},
	"DeleteApiKey": {Group: auth.GroupConfig},
}

func (s *ConfigServer) Get(
	ctx context.Context,
	req *connect.Request[metarrv1.ConfigServiceGetRequest],
) (*connect.Response[metarrv1.ConfigServiceGetResponse], error) {
	// The config document is a generated message now — the type the store
	// persists is the type this handler returns, so there is no conversion.
	// The response is a clone: it must carry blanked admin credentials, and
	// live config holds the running server's own password hash, so blanking
	// in place would erase it and lock the administrator out until the next
	// reload. UpdateAdmin stays the only write path for a new password.
	response := cloneMsg(appconfig.Get())
	if response.Admin != nil {
		response.Admin.PasswordSalt = ""
		response.Admin.PasswordHash = ""
	}

	return connect.NewResponse(&metarrv1.ConfigServiceGetResponse{
		Config: response,
	}), nil
}

func (s *ConfigServer) UpdateAdmin(
	ctx context.Context,
	req *connect.Request[metarrv1.ConfigServiceUpdateAdminRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	if req.Msg.Username != nil && req.Msg.GetUsername() == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("username cannot be empty"))
	}
	if req.Msg.Email != nil && req.Msg.GetEmail() == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("email cannot be empty"))
	}
	if req.Msg.Password != nil && req.Msg.GetPassword() == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("password cannot be empty"))
	}

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		if req.Msg.Username != nil {
			cfg.Admin.Username = req.Msg.GetUsername()
		}
		if req.Msg.Email != nil {
			cfg.Admin.Email = req.Msg.GetEmail()
		}
		if req.Msg.Password != nil {
			salt, hash, err := passwordhash.Hash(req.Msg.GetPassword())
			if err != nil {
				s.Logger.Error("failed to hash password", "correlation_id", correlationID, "error", err)
				return connectError(http.StatusInternalServerError, errors.New("failed to update admin credentials"))
			}
			cfg.Admin.PasswordSalt = salt
			cfg.Admin.PasswordHash = hash
		}
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

// UpsertApiKey replaces the entry addressed by group+entry.id, or creates
// one if entry.id is empty — minting the id at this RPC layer, the same
// "empty id creates" rule sidecar types use, before anything ever reaches
// the store. A non-empty id that names no existing entry is rejected rather
// than silently creating under it, matching UpsertSidecarType: otherwise an
// edit racing a delete of the same entry would resurrect it under its old
// id instead of failing. UpsertApiKey writes only cfg.ApiKeys, so an admin
// credential can never be part of what a key edit changes — see ADR 0001.
func (s *ConfigServer) UpsertApiKey(
	ctx context.Context,
	req *connect.Request[metarrv1.ConfigServiceUpsertApiKeyRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	group, err := appconfig.ParseAPIKeyGroup(req.Msg.GetGroup())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	// Cloned out of the inbound request: an entry kept in the persisted
	// config must not alias a message the RPC layer owns.
	entry := req.Msg.GetEntry()
	if entry == nil {
		entry = &appconfig.APIKeyEntry{}
	} else {
		entry = cloneMsg(entry)
	}
	creating := entry.Id == ""
	if creating {
		entry.Id = uuid.NewString()
	}

	mutateErr := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		if !creating && appconfig.FindAPIKeyIndex(cfg.ApiKeys, group, entry.Id) == -1 {
			return connectError(http.StatusNotFound, errors.New("no API key with that id"))
		}
		appconfig.UpsertAPIKey(cfg.ApiKeys, group, entry)
		return nil
	})
	if mutateErr != nil {
		return mutateConfigError(s.Logger, correlationID, mutateErr)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

// DeleteApiKey removes the entry addressed by group+id.
func (s *ConfigServer) DeleteApiKey(
	ctx context.Context,
	req *connect.Request[metarrv1.ConfigServiceDeleteApiKeyRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	group, err := appconfig.ParseAPIKeyGroup(req.Msg.GetGroup())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}
	id := req.Msg.GetId()

	mutateErr := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		if removed := appconfig.DeleteAPIKey(cfg.ApiKeys, group, id); !removed {
			return connectError(http.StatusNotFound, errors.New("no API key with that id"))
		}
		return nil
	})
	if mutateErr != nil {
		return mutateConfigError(s.Logger, correlationID, mutateErr)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}
