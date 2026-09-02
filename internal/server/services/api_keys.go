package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// ApiKeyServer implements metarrv1connect.ApiKeyServiceHandler: the API-key
// collection on AIP standard methods (docs/adr/0010) — minted-id addressed,
// Create / List scoped by the AccessLevel enum. Reads come from live config;
// every write goes through AppConfigStore.MutateSync, which persists under
// the store lock and propagates in-process before returning (docs/adr/0002).
// A write only ever touches cfg.ApiKeys, so an admin credential can never be
// part of what a key edit changes (ADR-0001).
type ApiKeyServer struct {
	*handlers.Handlers
}

// ApiKeyAuthPolicies is this service's method-name -> policy map. Every route
// is GroupConfig; the reads are read-only.
var ApiKeyAuthPolicies = map[string]httpserver.RPCPolicy{
	"CreateApiKey": {Group: auth.GroupConfig},
	"GetApiKey":    {Group: auth.GroupConfig, ReadOnly: true},
	"ListApiKeys":  {Group: auth.GroupConfig, ReadOnly: true},
	"UpdateApiKey": {Group: auth.GroupConfig},
	"DeleteApiKey": {Group: auth.GroupConfig},
}

// apiKeyOrderFields maps the order_by paths ListApiKeys accepts to their
// comparators; any other path is InvalidArgument.
var apiKeyOrderFields = map[string]func(a, b *metarrv1.APIKeyEntry) int{
	"name": func(a, b *metarrv1.APIKeyEntry) int { return strings.Compare(a.GetName(), b.GetName()) },
	"id":   func(a, b *metarrv1.APIKeyEntry) int { return strings.Compare(a.GetId(), b.GetId()) },
}

// accessLevelGroups maps each concrete AccessLevel to the storage group it
// addresses. ACCESS_LEVEL_UNSPECIFIED is deliberately absent — it resolves
// to InvalidArgument through resolveAccessLevel.
var accessLevelGroups = map[metarrv1.AccessLevel]appconfig.APIKeyGroup{
	metarrv1.AccessLevel_ACCESS_LEVEL_ADMIN:     appconfig.APIKeyGroupAdmin,
	metarrv1.AccessLevel_ACCESS_LEVEL_USER:      appconfig.APIKeyGroupUser,
	metarrv1.AccessLevel_ACCESS_LEVEL_WEBHOOK:   appconfig.APIKeyGroupWebhook,
	metarrv1.AccessLevel_ACCESS_LEVEL_READ_ONLY: appconfig.APIKeyGroupReadOnly,
}

// resolveAccessLevel turns a request's AccessLevel into the storage group it
// scopes to. An unset (ACCESS_LEVEL_UNSPECIFIED) or unknown value is
// InvalidArgument (docs/adr/0010).
func resolveAccessLevel(level metarrv1.AccessLevel) (appconfig.APIKeyGroup, error) {
	group, ok := accessLevelGroups[level]
	if !ok {
		return "", connectError(http.StatusBadRequest,
			fmt.Errorf("access_level must be one of ADMIN, USER, WEBHOOK, READ_ONLY; got %s", level))
	}
	return group, nil
}

func (s *ApiKeyServer) CreateApiKey(
	ctx context.Context,
	req *connect.Request[metarrv1.CreateApiKeyRequest],
) (*connect.Response[metarrv1.APIKeyEntry], error) {
	correlationID := correlation.FromContext(ctx)

	group, err := resolveAccessLevel(req.Msg.GetAccessLevel())
	if err != nil {
		return nil, err
	}

	entry := req.Msg.GetApiKey()
	if entry == nil {
		entry = &appconfig.APIKeyEntry{}
	} else {
		entry = cloneMsg(entry)
	}
	if entry.GetId() != "" {
		return nil, connectError(http.StatusBadRequest,
			errors.New("id is server-minted and must not be set on Create"))
	}

	var stored *appconfig.APIKeyEntry
	mutateErr := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		if cfg.ApiKeys == nil {
			cfg.ApiKeys = &appconfig.APIKeysConfig{}
		}
		entry.Id = uuid.NewString()
		appconfig.UpsertAPIKey(cfg.ApiKeys, group, entry)
		stored = cloneMsg(entry)
		return nil
	})
	if mutateErr != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, mutateErr)
	}
	return connect.NewResponse(stored), nil
}

func (s *ApiKeyServer) GetApiKey(
	ctx context.Context,
	req *connect.Request[metarrv1.GetApiKeyRequest],
) (*connect.Response[metarrv1.APIKeyEntry], error) {
	cfg := appconfig.Get()
	group, index := appconfig.FindAPIKeyByID(cfg.GetApiKeys(), req.Msg.GetId())
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no API key with that id"))
	}
	return connect.NewResponse(cloneMsg(appconfig.APIKeyEntriesFor(cfg.ApiKeys, group)[index])), nil
}

func (s *ApiKeyServer) ListApiKeys(
	ctx context.Context,
	req *connect.Request[metarrv1.ListApiKeysRequest],
) (*connect.Response[metarrv1.ListApiKeysResponse], error) {
	group, err := resolveAccessLevel(req.Msg.GetAccessLevel())
	if err != nil {
		return nil, err
	}
	if filterErr := parseListFilter(req.Msg.GetFilter()); filterErr != nil {
		return nil, aipConnectError(filterErr)
	}

	cfg := appconfig.Get()
	source := appconfig.APIKeyEntriesFor(cfg.GetApiKeys(), group)
	entries := make([]*metarrv1.APIKeyEntry, 0, len(source))
	for _, entry := range source {
		entries = append(entries, cloneMsg(entry))
	}

	if orderErr := orderBySlice(entries, req.Msg.GetOrderBy(), apiKeyOrderFields); orderErr != nil {
		return nil, aipConnectError(orderErr)
	}

	page, nextPageToken, pageErr := paginateSlice(entries, req.Msg.GetPageSize(), req.Msg.GetPageToken())
	if pageErr != nil {
		return nil, aipConnectError(pageErr)
	}
	return connect.NewResponse(&metarrv1.ListApiKeysResponse{
		ApiKeys:       page,
		NextPageToken: nextPageToken,
	}), nil
}

// UpdateApiKey is an AIP-134 partial update matched by minted id. update_mask
// may name only name / api_key; id is the addressing key and cannot be
// moved. An unknown id is NotFound, never a create.
func (s *ApiKeyServer) UpdateApiKey(
	ctx context.Context,
	req *connect.Request[metarrv1.UpdateApiKeyRequest],
) (*connect.Response[metarrv1.APIKeyEntry], error) {
	correlationID := correlation.FromContext(ctx)

	patch := req.Msg.GetApiKey()
	if patch == nil {
		return nil, connectError(http.StatusBadRequest, errors.New("api_key is required"))
	}
	id := patch.GetId()
	if id == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("api_key.id is required"))
	}
	for _, path := range req.Msg.GetUpdateMask().GetPaths() {
		if path == "id" {
			return nil, connectError(http.StatusBadRequest,
				fmt.Errorf("%w: id is the addressing key and cannot be changed", errUnknownPath))
		}
	}

	var stored *appconfig.APIKeyEntry
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		group, index := appconfig.FindAPIKeyByID(cfg.GetApiKeys(), id)
		if index == -1 {
			return connectError(http.StatusNotFound, errors.New("no API key with that id"))
		}
		entries := appconfig.APIKeyEntriesFor(cfg.ApiKeys, group)
		merged := cloneMsg(entries[index])
		if applyErr := applyUpdateMask(merged, patch, req.Msg.GetUpdateMask()); applyErr != nil {
			return applyErr
		}
		merged.Id = id
		entries[index] = merged
		stored = cloneMsg(merged)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(stored), nil
}

func (s *ApiKeyServer) DeleteApiKey(
	ctx context.Context,
	req *connect.Request[metarrv1.DeleteApiKeyRequest],
) (*connect.Response[emptypb.Empty], error) {
	correlationID := correlation.FromContext(ctx)

	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		group, index := appconfig.FindAPIKeyByID(cfg.GetApiKeys(), req.Msg.GetId())
		if index == -1 {
			return connectError(http.StatusNotFound, errors.New("no API key with that id"))
		}
		if !appconfig.DeleteAPIKey(cfg.ApiKeys, group, req.Msg.GetId()) {
			return connectError(http.StatusNotFound, errors.New("no API key with that id"))
		}
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}
