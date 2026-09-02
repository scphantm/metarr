package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// SonarrInterfaceServer implements
// metarrv1connect.SonarrInterfaceServiceHandler: the Sonarr instance
// collection on AIP standard methods (docs/adr/0010). Reads come from live
// config; every write goes through AppConfigStore.MutateSync, which persists
// under the store lock and propagates in-process before returning
// (docs/adr/0002).
type SonarrInterfaceServer struct {
	*handlers.Handlers
}

// SonarrInterfaceAuthPolicies is this service's method-name -> policy map,
// passed to httpserver.NewConnectAuthInterceptor when mounting it. Every
// route is GroupConfig; the two reads are read-only.
var SonarrInterfaceAuthPolicies = map[string]httpserver.RPCPolicy{
	"CreateSonarrInstance": {Group: auth.GroupConfig},
	"GetSonarrInstance":    {Group: auth.GroupConfig, ReadOnly: true},
	"ListSonarrInstances":  {Group: auth.GroupConfig, ReadOnly: true},
	"UpdateSonarrInstance": {Group: auth.GroupConfig},
	"DeleteSonarrInstance": {Group: auth.GroupConfig},
}

// sonarrOrderFields maps the order_by paths ListSonarrInstances accepts to
// their comparators; any other path is InvalidArgument.
var sonarrOrderFields = map[string]func(a, b *metarrv1.SonarrInstance) int{
	"instance_slug": func(a, b *metarrv1.SonarrInstance) int {
		return strings.Compare(a.GetInstanceSlug(), b.GetInstanceSlug())
	},
	"instance_name": func(a, b *metarrv1.SonarrInstance) int {
		return strings.Compare(a.GetInstanceName(), b.GetInstanceName())
	},
}

func (s *SonarrInterfaceServer) CreateSonarrInstance(
	ctx context.Context,
	req *connect.Request[metarrv1.CreateSonarrInstanceRequest],
) (*connect.Response[metarrv1.SonarrInstance], error) {
	correlationID := correlation.FromContext(ctx)

	slug := req.Msg.GetSonarrInstanceId()
	instance := cloneOrNewSonarrInstance(req.Msg.GetSonarrInstance())
	// AIP-133: a slug in the body must match sonarr_instance_id or be empty.
	if bodySlug := instance.GetInstanceSlug(); bodySlug != "" && bodySlug != slug {
		return nil, connectError(http.StatusBadRequest,
			fmt.Errorf("sonarr_instance.instance_slug %q does not match sonarr_instance_id %q", bodySlug, slug))
	}
	instance.InstanceSlug = slug

	var stored *metarrv1.SonarrInstance
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		created, err := appendNewSonarrInstance(cfg, instance)
		stored = created
		return err
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(stored), nil
}

func (s *SonarrInterfaceServer) GetSonarrInstance(
	ctx context.Context,
	req *connect.Request[metarrv1.GetSonarrInstanceRequest],
) (*connect.Response[metarrv1.SonarrInstance], error) {
	cfg := appconfig.Get()
	index := appconfig.FindSonarrIndex(cfg.Interfaces, req.Msg.GetSlug())
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no Sonarr instance with that slug"))
	}
	return connect.NewResponse(cloneMsg(cfg.Interfaces.Sonarr[index])), nil
}

func (s *SonarrInterfaceServer) ListSonarrInstances(
	ctx context.Context,
	req *connect.Request[metarrv1.ListSonarrInstancesRequest],
) (*connect.Response[metarrv1.ListSonarrInstancesResponse], error) {
	if err := parseListFilter(req.Msg.GetFilter()); err != nil {
		return nil, aipConnectError(err)
	}

	cfg := appconfig.Get()
	instances := make([]*metarrv1.SonarrInstance, 0, len(cfg.Interfaces.Sonarr))
	for _, instance := range cfg.Interfaces.Sonarr {
		instances = append(instances, cloneMsg(instance))
	}

	if err := orderBySlice(instances, req.Msg.GetOrderBy(), sonarrOrderFields); err != nil {
		return nil, aipConnectError(err)
	}

	page, nextPageToken, err := paginateSlice(instances, req.Msg.GetPageSize(), req.Msg.GetPageToken())
	if err != nil {
		return nil, aipConnectError(err)
	}
	return connect.NewResponse(&metarrv1.ListSonarrInstancesResponse{
		SonarrInstances: page,
		NextPageToken:   nextPageToken,
	}), nil
}

func (s *SonarrInterfaceServer) UpdateSonarrInstance(
	ctx context.Context,
	req *connect.Request[metarrv1.UpdateSonarrInstanceRequest],
) (*connect.Response[metarrv1.SonarrInstance], error) {
	correlationID := correlation.FromContext(ctx)

	patch := req.Msg.GetSonarrInstance()
	if patch == nil {
		return nil, connectError(http.StatusBadRequest, errors.New("sonarr_instance is required"))
	}
	slug := patch.GetInstanceSlug()
	if slug == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("sonarr_instance.instance_slug is required"))
	}

	var stored *metarrv1.SonarrInstance
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		index := appconfig.FindSonarrIndex(cfg.Interfaces, slug)
		if index == -1 {
			// allow_missing:true upgrades an Update on an unknown slug to a
			// Create — the mask is ignored and the whole resource is
			// validated as a Create (docs/adr/0010).
			if !req.Msg.GetAllowMissing() {
				return connectError(http.StatusNotFound, errors.New("no Sonarr instance with that slug"))
			}
			created, err := appendNewSonarrInstance(cfg, cloneMsg(patch))
			stored = created
			return err
		}

		merged := cloneMsg(cfg.Interfaces.Sonarr[index])
		if err := applyUpdateMask(merged, patch, req.Msg.GetUpdateMask()); err != nil {
			return err
		}
		// The mask cannot move the slug — it is the addressing key.
		merged.InstanceSlug = slug
		ensureStorageSection(merged)
		if err := validateSonarrInstance(merged); err != nil {
			return connectError(http.StatusBadRequest, err)
		}
		cfg.Interfaces.Sonarr[index] = merged
		stored = cloneMsg(merged)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(stored), nil
}

func (s *SonarrInterfaceServer) DeleteSonarrInstance(
	ctx context.Context,
	req *connect.Request[metarrv1.DeleteSonarrInstanceRequest],
) (*connect.Response[emptypb.Empty], error) {
	correlationID := correlation.FromContext(ctx)

	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		index := appconfig.FindSonarrIndex(cfg.Interfaces, req.Msg.GetSlug())
		if index == -1 {
			return connectError(http.StatusNotFound, errors.New("no Sonarr instance with that slug"))
		}
		cfg.Interfaces.Sonarr = append(cfg.Interfaces.Sonarr[:index], cfg.Interfaces.Sonarr[index+1:]...)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

// cloneOrNewSonarrInstance returns a detached copy of the request's instance
// with a non-nil storage section, or a fresh one when the request carried
// none — a stored instance always has a storage section so the cache can
// read its retention mode without a nil check (appconfig.Normalize backfills
// it on every later read; this keeps it non-nil in between).
func cloneOrNewSonarrInstance(instance *metarrv1.SonarrInstance) *metarrv1.SonarrInstance {
	if instance == nil {
		instance = &metarrv1.SonarrInstance{}
	} else {
		instance = cloneMsg(instance)
	}
	ensureStorageSection(instance)
	return instance
}

func ensureStorageSection(instance *metarrv1.SonarrInstance) {
	if instance.Storage == nil {
		instance.Storage = &metarrv1.StorageConfig{}
	}
}

// appendNewSonarrInstance validates instance as a whole resource and adds it
// to cfg, enforcing both not-found rules: an existing Sonarr slug is
// AlreadyExists, a slug held by another interface type is FailedPrecondition.
// It is the shared body of CreateSonarrInstance and the allow_missing branch
// of UpdateSonarrInstance, which ADR-0010 says is "validated as a Create". It
// returns the stored clone; the closures that call it run under the store
// lock.
func appendNewSonarrInstance(cfg *appconfig.Config, instance *metarrv1.SonarrInstance) (*metarrv1.SonarrInstance, error) {
	ensureStorageSection(instance)
	if err := validateSonarrInstance(instance); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}
	slug := instance.GetInstanceSlug()
	if appconfig.FindSonarrIndex(cfg.Interfaces, slug) != -1 {
		return nil, connectError(http.StatusConflict,
			fmt.Errorf("a Sonarr instance with slug %q already exists", slug))
	}
	if err := checkSlugFreeAcrossInterfaces(cfg, slug); err != nil {
		return nil, err
	}
	cfg.Interfaces.Sonarr = append(cfg.Interfaces.Sonarr, instance)
	return cloneMsg(instance), nil
}

// validateSonarrInstance is the whole-resource check a Create (and an
// allow_missing Update that creates) runs before the store writes.
func validateSonarrInstance(instance *metarrv1.SonarrInstance) error {
	if instance.GetInstanceSlug() == "" {
		return errors.New("instance_slug is required")
	}
	return nil
}

// checkSlugFreeAcrossInterfaces enforces the cross-interface-type slug
// uniqueness rule on the write path: a slug already in use by any interface
// type (today only Sonarr) is rejected as FailedPrecondition — a state
// conflict rather than a bad argument, and distinct from the AlreadyExists a
// same-collection Create clash returns (docs/adr/0010).
func checkSlugFreeAcrossInterfaces(cfg *appconfig.Config, slug string) error {
	for _, used := range appconfig.AllInstanceSlugs(cfg.Interfaces) {
		if used == slug {
			return connectError(http.StatusUnprocessableEntity,
				fmt.Errorf("slug %q is already in use by another interface type", slug))
		}
	}
	return nil
}
