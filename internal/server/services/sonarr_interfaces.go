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

// SonarrInterfaceServer implements metarrv1connect.SonarrInterfaceServiceHandler.
// Every write goes through AppConfigStore.Mutate — see
// internal/server/appconfigstore.
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
	appConfig := appconfig.Get()

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
	appConfig := appconfig.Get()

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

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		index := cfg.Interfaces.FindSonarrIndex(instance.InstanceSlug)
		if index == -1 {
			for _, slug := range cfg.Interfaces.AllInstanceSlugs() {
				if slug == instance.InstanceSlug {
					return connectError(http.StatusConflict, errors.New("instance_slug already in use by a different interface type"))
				}
			}
			cfg.Interfaces.Sonarr = append(cfg.Interfaces.Sonarr, instance)
		} else {
			cfg.Interfaces.Sonarr[index] = instance
		}
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *SonarrInterfaceServer) Delete(
	ctx context.Context,
	req *connect.Request[metarrv1.SonarrInterfaceServiceDeleteRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		index := cfg.Interfaces.FindSonarrIndex(req.Msg.GetSlug())
		if index == -1 {
			return connectError(http.StatusNotFound, errors.New("no Sonarr instance with that slug"))
		}
		cfg.Interfaces.Sonarr = append(cfg.Interfaces.Sonarr[:index], cfg.Interfaces.Sonarr[index+1:]...)
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
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
