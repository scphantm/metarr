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
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// SonarrInterfaceServer implements metarrv1connect.SonarrInterfaceServiceHandler,
// ported directly from internal/server/handlers/sonarr_interfaces.go — same
// Mongo reads and FireConfigUpdate call, only the transport changed.
type SonarrInterfaceServer struct {
	*handlers.Handlers
}

// SonarrInterfaceAuthPolicies is this service's method-name -> policy map,
// passed to httpserver.NewConnectAuthInterceptor when mounting it. Mirrors
// every route in router.go being GroupConfig; List/Get are read-only.
var SonarrInterfaceAuthPolicies = map[string]httpserver.RPCPolicy{
	"List":   {Group: auth.GroupConfig, ReadOnly: true},
	"Get":    {Group: auth.GroupConfig, ReadOnly: true},
	"Upsert": {Group: auth.GroupConfig},
	"Delete": {Group: auth.GroupConfig},
}

func (s *SonarrInterfaceServer) List(
	ctx context.Context,
	req *connect.Request[metarrv1.SonarrInterfaceServiceListRequest],
) (*connect.Response[metarrv1.SonarrInterfaceServiceListResponse], error) {
	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	instances := make([]*metarrv1.SonarrInstance, 0, len(appConfig.Interfaces.Sonarr))
	for _, instance := range appConfig.Interfaces.Sonarr {
		instances = append(instances, sonarrInstanceToProto(instance))
	}
	return connect.NewResponse(&metarrv1.SonarrInterfaceServiceListResponse{Instances: instances}), nil
}

func (s *SonarrInterfaceServer) Get(
	ctx context.Context,
	req *connect.Request[metarrv1.SonarrInterfaceServiceGetRequest],
) (*connect.Response[metarrv1.SonarrInterfaceServiceGetResponse], error) {
	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	index := appConfig.Interfaces.FindSonarrIndex(req.Msg.GetSlug())
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no Sonarr instance with that slug"))
	}
	return connect.NewResponse(&metarrv1.SonarrInterfaceServiceGetResponse{
		Instance: sonarrInstanceToProto(appConfig.Interfaces.Sonarr[index]),
	}), nil
}

func (s *SonarrInterfaceServer) Upsert(
	ctx context.Context,
	req *connect.Request[metarrv1.SonarrInterfaceServiceUpsertRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	instance := sonarrInstanceFromProto(req.Msg.GetInstance())
	if instance.InstanceSlug == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("instance_slug is required"))
	}

	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	index := appConfig.Interfaces.FindSonarrIndex(instance.InstanceSlug)
	if index == -1 {
		for _, slug := range appConfig.Interfaces.AllInstanceSlugs() {
			if slug == instance.InstanceSlug {
				return nil, connectError(http.StatusConflict, errors.New("instance_slug already in use by a different interface type"))
			}
		}
		appConfig.Interfaces.Sonarr = append(appConfig.Interfaces.Sonarr, instance)
	} else {
		appConfig.Interfaces.Sonarr[index] = instance
	}

	if err := s.FireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		s.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue config update"))
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *SonarrInterfaceServer) Delete(
	ctx context.Context,
	req *connect.Request[metarrv1.SonarrInterfaceServiceDeleteRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	index := appConfig.Interfaces.FindSonarrIndex(req.Msg.GetSlug())
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no Sonarr instance with that slug"))
	}
	appConfig.Interfaces.Sonarr = append(appConfig.Interfaces.Sonarr[:index], appConfig.Interfaces.Sonarr[index+1:]...)

	if err := s.FireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		s.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue config update"))
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func sonarrInstanceToProto(instance appconfig.SonarrInstance) *metarrv1.SonarrInstance {
	mappings := make([]*metarrv1.RootDirMapping, 0, len(instance.RootDirMap))
	for _, m := range instance.RootDirMap {
		mappings = append(mappings, &metarrv1.RootDirMapping{
			SonarrPath: m.SonarrPath,
			LocalPath:  m.LocalPath,
		})
	}
	return &metarrv1.SonarrInstance{
		InstanceName: instance.InstanceName,
		InstanceSlug: instance.InstanceSlug,
		SonarrUrl:    instance.SonarrURL,
		SonarrApiKey: instance.SonarrAPIKey,
		RootDirMap:   mappings,
		Storage: &metarrv1.StorageConfig{
			Mode:     instance.Storage.Mode,
			Ttl:      instance.Storage.TTL,
			MaxCount: int32(instance.Storage.MaxCount),
		},
	}
}

func sonarrInstanceFromProto(instance *metarrv1.SonarrInstance) appconfig.SonarrInstance {
	mappings := make([]appconfig.RootDirMapping, 0, len(instance.GetRootDirMap()))
	for _, m := range instance.GetRootDirMap() {
		mappings = append(mappings, appconfig.RootDirMapping{
			SonarrPath: m.GetSonarrPath(),
			LocalPath:  m.GetLocalPath(),
		})
	}
	storage := instance.GetStorage()
	return appconfig.SonarrInstance{
		InstanceName: instance.GetInstanceName(),
		InstanceSlug: instance.GetInstanceSlug(),
		SonarrURL:    instance.GetSonarrUrl(),
		SonarrAPIKey: instance.GetSonarrApiKey(),
		RootDirMap:   mappings,
		Storage: appconfig.StorageConfig{
			Mode:     storage.GetMode(),
			TTL:      storage.GetTtl(),
			MaxCount: int(storage.GetMaxCount()),
		},
	}
}
