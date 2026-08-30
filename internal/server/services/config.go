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
	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	// Redaction happens implicitly: configToProto's AdminUser conversion
	// never reads PasswordSalt/PasswordHash off the Go struct at all — see
	// config.proto's AdminUser doc comment.
	return connect.NewResponse(&metarrv1.ConfigServiceGetResponse{
		Config: configToProto(appConfig),
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
// id instead of failing. UpsertApiKey writes only cfg.APIKeys, so an admin
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

	entry := apiKeyEntryFromProto(req.Msg.GetEntry())
	creating := entry.ID == ""
	if creating {
		entry.ID = uuid.NewString()
	}

	mutateErr := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		if !creating && cfg.APIKeys.FindAPIKeyIndex(group, entry.ID) == -1 {
			return connectError(http.StatusNotFound, errors.New("no API key with that id"))
		}
		cfg.APIKeys.UpsertAPIKey(group, entry)
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
		if removed := cfg.APIKeys.DeleteAPIKey(group, id); !removed {
			return connectError(http.StatusNotFound, errors.New("no API key with that id"))
		}
		return nil
	})
	if mutateErr != nil {
		return mutateConfigError(s.Logger, correlationID, mutateErr)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func apiKeyEntryFromProto(entry *metarrv1.APIKeyEntry) appconfig.APIKeyEntry {
	return appconfig.APIKeyEntry{ID: entry.GetId(), Name: entry.GetName(), Key: entry.GetApiKey()}
}

func configToProto(config *appconfig.Config) *metarrv1.Config {
	return &metarrv1.Config{
		ApiKeys: apiKeysConfigToProto(config.APIKeys),
		Admin: &metarrv1.AdminUser{
			Username: config.Admin.Username,
			Email:    config.Admin.Email,
		},
		Interfaces:       interfacesConfigToProto(config.Interfaces),
		DirectoryScanner: directoryScannerConfigToProto(config.DirectoryScanner),
		Agents:           agentConfigsToProto(config.Agents),
		Logging: &metarrv1.LoggingConfig{
			ServerLevel: config.Logging.ServerLevel,
			Sink:        config.Logging.Sink,
			Endpoint:    config.Logging.Endpoint,
			Stream:      config.Logging.Stream,
		},
	}
}

func apiKeysConfigToProto(keys appconfig.APIKeysConfig) *metarrv1.APIKeysConfig {
	return &metarrv1.APIKeysConfig{
		Admin:    apiKeyEntriesToProto(keys.Admin),
		User:     apiKeyEntriesToProto(keys.User),
		Webhook:  apiKeyEntriesToProto(keys.Webhook),
		ReadOnly: apiKeyEntriesToProto(keys.ReadOnly),
	}
}

func apiKeyEntriesToProto(entries []appconfig.APIKeyEntry) []*metarrv1.APIKeyEntry {
	out := make([]*metarrv1.APIKeyEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, &metarrv1.APIKeyEntry{Id: entry.ID, Name: entry.Name, ApiKey: entry.Key})
	}
	return out
}

func interfacesConfigToProto(interfaces appconfig.InterfacesConfig) *metarrv1.InterfacesConfig {
	sonarr := make([]*metarrv1.SonarrInstance, 0, len(interfaces.Sonarr))
	for _, instance := range interfaces.Sonarr {
		sonarr = append(sonarr, sonarrInstanceToProto(instance))
	}
	return &metarrv1.InterfacesConfig{Sonarr: sonarr}
}

func directoryScannerConfigToProto(scanner appconfig.DirectoryScannerConfig) *metarrv1.DirectoryScannerConfig {
	dirs := make([]*metarrv1.ScanDirectory, 0, len(scanner.ScanDirectories))
	for _, dir := range scanner.ScanDirectories {
		dirs = append(dirs, scanDirectoryToProto(dir))
	}
	types := make([]*metarrv1.SidecarTypeDefinition, 0, len(scanner.SidecarTypes))
	for _, def := range scanner.SidecarTypes {
		types = append(types, sidecarTypeDefinitionToProto(def))
	}
	return &metarrv1.DirectoryScannerConfig{
		ParallelCount:   int32(scanner.ParallelCount),
		ScanDirectories: dirs,
		SidecarTypes:    types,
	}
}

func scanDirectoryToProto(dir appconfig.ScanDirectory) *metarrv1.ScanDirectory {
	return &metarrv1.ScanDirectory{
		ScannerSlug: dir.ScannerSlug,
		ScanType:    dir.ScanType,
		Directory:   dir.Directory,
	}
}

func scanDirectoryFromProto(dir *metarrv1.ScanDirectory) appconfig.ScanDirectory {
	return appconfig.ScanDirectory{
		ScannerSlug: dir.GetScannerSlug(),
		ScanType:    dir.GetScanType(),
		Directory:   dir.GetDirectory(),
	}
}

func sidecarTypeDefinitionToProto(def appconfig.SidecarTypeDefinition) *metarrv1.SidecarTypeDefinition {
	return &metarrv1.SidecarTypeDefinition{
		Id:         def.ID,
		Type:       def.Type,
		Category:   def.Category,
		Order:      int32(def.Order),
		Patterns:   def.Patterns,
		Extensions: def.Extensions,
	}
}

func sidecarTypeDefinitionFromProto(def *metarrv1.SidecarTypeDefinition) appconfig.SidecarTypeDefinition {
	return appconfig.SidecarTypeDefinition{
		ID:         def.GetId(),
		Type:       def.GetType(),
		Category:   def.GetCategory(),
		Order:      int(def.GetOrder()),
		Patterns:   def.GetPatterns(),
		Extensions: def.GetExtensions(),
	}
}

func agentConfigsToProto(agents []appconfig.AgentConfig) []*metarrv1.AgentConfig {
	out := make([]*metarrv1.AgentConfig, 0, len(agents))
	for _, agent := range agents {
		mappings := make([]*metarrv1.AgentDirectoryMapping, 0, len(agent.Mappings))
		for _, m := range agent.Mappings {
			mappings = append(mappings, &metarrv1.AgentDirectoryMapping{
				ScannerSlug: m.ScannerSlug,
				AgentPath:   m.AgentPath,
			})
		}
		out = append(out, &metarrv1.AgentConfig{
			Slug:        agent.Slug,
			DisplayName: agent.DisplayName,
			Mappings:    mappings,
			LogLevel:    agent.LogLevel,
		})
	}
	return out
}
