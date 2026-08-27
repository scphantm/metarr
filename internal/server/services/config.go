package services

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/server/passwordhash"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// ConfigServer implements metarrv1connect.ConfigServiceHandler, ported
// directly from internal/server/handlers/config.go (Get/Update) and
// internal/server/handlers/admin.go (UpdateAdmin) — same Mongo reads and
// FireConfigUpdate call, only the transport changed.
type ConfigServer struct {
	*handlers.Handlers
}

// ConfigAuthPolicies is this service's method-name -> policy map. Mirrors
// every config route in router.go being GroupConfig.
var ConfigAuthPolicies = map[string]httpserver.RPCPolicy{
	"Get":         {Group: auth.GroupConfig, ReadOnly: true},
	"Update":      {Group: auth.GroupConfig},
	"UpdateAdmin": {Group: auth.GroupConfig},
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

func (s *ConfigServer) Update(
	ctx context.Context,
	req *connect.Request[metarrv1.ConfigServiceUpdateRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	updatedConfig := configFromProto(req.Msg.GetConfig())

	if err := s.FireConfigUpdate(ctx, correlationID, updatedConfig); err != nil {
		s.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue config update"))
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
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

	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	if req.Msg.Username != nil {
		appConfig.Admin.Username = req.Msg.GetUsername()
	}
	if req.Msg.Email != nil {
		appConfig.Admin.Email = req.Msg.GetEmail()
	}
	if req.Msg.Password != nil {
		salt, hash, err := passwordhash.Hash(req.Msg.GetPassword())
		if err != nil {
			s.Logger.Error("failed to hash password", "correlation_id", correlationID, "error", err)
			return nil, connectError(http.StatusInternalServerError, errors.New("failed to update admin credentials"))
		}
		appConfig.Admin.PasswordSalt = salt
		appConfig.Admin.PasswordHash = hash
	}

	if err := s.FireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		s.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue config update"))
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
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

// configFromProto converts a wire Config back into appconfig.Config.
// PasswordSalt/PasswordHash are never set here — the proto AdminUser has no
// such fields, so a whole-document Update leaves them at Go's zero value,
// exactly matching what the REST UpdateConfig handler already did with a
// JSON body carrying the same (GetConfig-redacted) empty strings.
func configFromProto(config *metarrv1.Config) appconfig.Config {
	admin := config.GetAdmin()
	return appconfig.Config{
		APIKeys: apiKeysConfigFromProto(config.GetApiKeys()),
		Admin: appconfig.AdminUser{
			Username: admin.GetUsername(),
			Email:    admin.GetEmail(),
		},
		Interfaces:       interfacesConfigFromProto(config.GetInterfaces()),
		DirectoryScanner: directoryScannerConfigFromProto(config.GetDirectoryScanner()),
		Agents:           agentConfigsFromProto(config.GetAgents()),
		Logging: appconfig.LoggingConfig{
			ServerLevel: config.GetLogging().GetServerLevel(),
			Sink:        config.GetLogging().GetSink(),
			Endpoint:    config.GetLogging().GetEndpoint(),
			Stream:      config.GetLogging().GetStream(),
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

func apiKeysConfigFromProto(keys *metarrv1.APIKeysConfig) appconfig.APIKeysConfig {
	return appconfig.APIKeysConfig{
		Admin:    apiKeyEntriesFromProto(keys.GetAdmin()),
		User:     apiKeyEntriesFromProto(keys.GetUser()),
		Webhook:  apiKeyEntriesFromProto(keys.GetWebhook()),
		ReadOnly: apiKeyEntriesFromProto(keys.GetReadOnly()),
	}
}

func apiKeyEntriesToProto(entries []appconfig.APIKeyEntry) []*metarrv1.APIKeyEntry {
	out := make([]*metarrv1.APIKeyEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, &metarrv1.APIKeyEntry{Name: entry.Name, ApiKey: entry.Key})
	}
	return out
}

func apiKeyEntriesFromProto(entries []*metarrv1.APIKeyEntry) []appconfig.APIKeyEntry {
	out := make([]appconfig.APIKeyEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, appconfig.APIKeyEntry{Name: entry.GetName(), Key: entry.GetApiKey()})
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

func interfacesConfigFromProto(interfaces *metarrv1.InterfacesConfig) appconfig.InterfacesConfig {
	sonarr := make([]appconfig.SonarrInstance, 0, len(interfaces.GetSonarr()))
	for _, instance := range interfaces.GetSonarr() {
		sonarr = append(sonarr, sonarrInstanceFromProto(instance))
	}
	return appconfig.InterfacesConfig{Sonarr: sonarr}
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

func directoryScannerConfigFromProto(scanner *metarrv1.DirectoryScannerConfig) appconfig.DirectoryScannerConfig {
	dirs := make([]appconfig.ScanDirectory, 0, len(scanner.GetScanDirectories()))
	for _, dir := range scanner.GetScanDirectories() {
		dirs = append(dirs, scanDirectoryFromProto(dir))
	}
	types := make([]appconfig.SidecarTypeDefinition, 0, len(scanner.GetSidecarTypes()))
	for _, def := range scanner.GetSidecarTypes() {
		types = append(types, sidecarTypeDefinitionFromProto(def))
	}
	return appconfig.DirectoryScannerConfig{
		ParallelCount:   int(scanner.GetParallelCount()),
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

func agentConfigsFromProto(agents []*metarrv1.AgentConfig) []appconfig.AgentConfig {
	out := make([]appconfig.AgentConfig, 0, len(agents))
	for _, agent := range agents {
		out = append(out, agentConfigFromProto(agent))
	}
	return out
}
