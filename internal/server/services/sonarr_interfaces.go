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
		instances = append(instances, cloneMsg(instance))
	}
	return connect.NewResponse(&metarrv1.SonarrInterfaceServiceListResponse{Instances: instances}), nil
}

func (s *SonarrInterfaceServer) Get(
	ctx context.Context,
	req *connect.Request[metarrv1.SonarrInterfaceServiceGetRequest],
) (*connect.Response[metarrv1.SonarrInterfaceServiceGetResponse], error) {
	appConfig := appconfig.Get()

	index := appconfig.FindSonarrIndex(appConfig.Interfaces, req.Msg.GetSlug())
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no Sonarr instance with that slug"))
	}
	return connect.NewResponse(&metarrv1.SonarrInterfaceServiceGetResponse{
		Instance: cloneMsg(appConfig.Interfaces.Sonarr[index]),
	}), nil
}

func (s *SonarrInterfaceServer) Upsert(
	ctx context.Context,
	req *connect.Request[metarrv1.SonarrInterfaceServiceUpsertRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	instance := req.Msg.GetInstance()
	if instance == nil {
		instance = &appconfig.SonarrInstance{}
	} else {
		instance = cloneMsg(instance)
	}
	// A stored instance always carries a storage section so the cache can
	// read its retention mode without a nil check; appconfig.Normalize
	// backfills it on every later read, this keeps it non-nil in between.
	if instance.Storage == nil {
		instance.Storage = &appconfig.StorageConfig{}
	}
	if instance.InstanceSlug == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("instance_slug is required"))
	}

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		index := appconfig.FindSonarrIndex(cfg.Interfaces, instance.InstanceSlug)
		if index == -1 {
			for _, slug := range appconfig.AllInstanceSlugs(cfg.Interfaces) {
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
		index := appconfig.FindSonarrIndex(cfg.Interfaces, req.Msg.GetSlug())
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
