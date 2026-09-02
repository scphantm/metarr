package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/scanmodel"
)

// DirectoryScannerServer implements metarrv1connect.DirectoryScannerServiceHandler.
// Every write goes through AppConfigStore.Mutate — see
// internal/server/appconfigstore.
type DirectoryScannerServer struct {
	*handlers.Handlers
}

// DirectoryScannerAuthPolicies is this service's method-name -> policy map.
// Mirrors every directory-scanner route in router.go being GroupConfig.
var DirectoryScannerAuthPolicies = map[string]httpserver.RPCPolicy{
	"Get":                 {Group: auth.GroupConfig, ReadOnly: true},
	"Update":              {Group: auth.GroupConfig},
	"ListDirectories":     {Group: auth.GroupConfig, ReadOnly: true},
	"GetDirectory":        {Group: auth.GroupConfig, ReadOnly: true},
	"UpsertDirectory":     {Group: auth.GroupConfig},
	"DeleteDirectory":     {Group: auth.GroupConfig},
	"ListSidecarTypes":    {Group: auth.GroupConfig, ReadOnly: true},
	"GetSidecarType":      {Group: auth.GroupConfig, ReadOnly: true},
	"UpsertSidecarType":   {Group: auth.GroupConfig},
	"DeleteSidecarType":   {Group: auth.GroupConfig},
	"ReorderSidecarTypes": {Group: auth.GroupConfig},
	"ResetSidecarTypes":   {Group: auth.GroupConfig},
}

func (s *DirectoryScannerServer) Get(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceGetRequest],
) (*connect.Response[metarrv1.DirectoryScannerServiceGetResponse], error) {
	appConfig := appconfig.Get()
	return connect.NewResponse(&metarrv1.DirectoryScannerServiceGetResponse{
		Config: cloneMsg(appConfig.DirectoryScanner),
	}), nil
}

func (s *DirectoryScannerServer) Update(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceUpdateRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	if req.Msg.ParallelCount != nil && req.Msg.GetParallelCount() <= 0 {
		return nil, connectError(http.StatusBadRequest, errors.New("parallel_count must be greater than zero"))
	}

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		if req.Msg.ParallelCount != nil {
			cfg.DirectoryScanner.ParallelCount = req.Msg.GetParallelCount()
		}
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *DirectoryScannerServer) ListDirectories(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceListDirectoriesRequest],
) (*connect.Response[metarrv1.DirectoryScannerServiceListDirectoriesResponse], error) {
	appConfig := appconfig.Get()

	dirs := make([]*metarrv1.ScanDirectory, 0, len(appConfig.DirectoryScanner.ScanDirectories))
	for _, dir := range appConfig.DirectoryScanner.ScanDirectories {
		dirs = append(dirs, cloneMsg(dir))
	}
	return connect.NewResponse(&metarrv1.DirectoryScannerServiceListDirectoriesResponse{Directories: dirs}), nil
}

func (s *DirectoryScannerServer) GetDirectory(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceGetDirectoryRequest],
) (*connect.Response[metarrv1.DirectoryScannerServiceGetDirectoryResponse], error) {
	appConfig := appconfig.Get()

	index := appconfig.FindScanDirectoryIndex(appConfig.DirectoryScanner, req.Msg.GetSlug())
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no scan directory with that slug"))
	}
	return connect.NewResponse(&metarrv1.DirectoryScannerServiceGetDirectoryResponse{
		Directory: cloneMsg(appConfig.DirectoryScanner.ScanDirectories[index]),
	}), nil
}

func (s *DirectoryScannerServer) UpsertDirectory(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceUpsertDirectoryRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	entry := req.Msg.GetDirectory()
	if entry == nil {
		entry = &appconfig.ScanDirectory{}
	} else {
		entry = cloneMsg(entry)
	}
	if entry.ScannerSlug == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("scanner_slug is required"))
	}
	// Reject an unscannable scan_type here rather than letting it fail later in
	// the scan listener, where nobody is waiting to see the error.
	if _, err := scanmodel.ParseDirectoryType(entry.ScanType); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		if index := appconfig.FindScanDirectoryIndex(cfg.DirectoryScanner, entry.ScannerSlug); index == -1 {
			cfg.DirectoryScanner.ScanDirectories = append(cfg.DirectoryScanner.ScanDirectories, entry)
		} else {
			cfg.DirectoryScanner.ScanDirectories[index] = entry
		}
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *DirectoryScannerServer) DeleteDirectory(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceDeleteDirectoryRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)
	slug := req.Msg.GetSlug()

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		index := appconfig.FindScanDirectoryIndex(cfg.DirectoryScanner, slug)
		if index == -1 {
			return connectError(http.StatusNotFound, errors.New("no scan directory with that slug"))
		}
		scanDirectories := cfg.DirectoryScanner.ScanDirectories
		cfg.DirectoryScanner.ScanDirectories = append(scanDirectories[:index], scanDirectories[index+1:]...)
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *DirectoryScannerServer) ListSidecarTypes(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceListSidecarTypesRequest],
) (*connect.Response[metarrv1.DirectoryScannerServiceListSidecarTypesResponse], error) {
	appConfig := appconfig.Get()

	types := make([]*appconfig.SidecarTypeDefinition, 0, len(appConfig.DirectoryScanner.SidecarTypes))
	for _, def := range appConfig.DirectoryScanner.SidecarTypes {
		types = append(types, cloneMsg(def))
	}
	return connect.NewResponse(&metarrv1.DirectoryScannerServiceListSidecarTypesResponse{Types: types}), nil
}

func (s *DirectoryScannerServer) GetSidecarType(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceGetSidecarTypeRequest],
) (*connect.Response[metarrv1.DirectoryScannerServiceGetSidecarTypeResponse], error) {
	appConfig := appconfig.Get()

	index := appconfig.FindSidecarTypeIndexByID(appConfig.DirectoryScanner, req.Msg.GetId())
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no sidecar type with that id"))
	}
	return connect.NewResponse(&metarrv1.DirectoryScannerServiceGetSidecarTypeResponse{
		Type: cloneMsg(appConfig.DirectoryScanner.SidecarTypes[index]),
	}), nil
}

func (s *DirectoryScannerServer) UpsertSidecarType(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceUpsertSidecarTypeRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	entry := req.Msg.GetType()
	if entry == nil {
		entry = &appconfig.SidecarTypeDefinition{}
	} else {
		entry = cloneMsg(entry)
	}
	if entry.Type == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("type is required"))
	}
	if _, err := scanmodel.ParseSidecarCategory(entry.Category); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}
	// Rejected rather than ignored: silently dropping an order someone took the
	// trouble to send would leave them believing they had reordered the table.
	if entry.Order != 0 {
		return nil, connectError(http.StatusBadRequest, errors.New("order cannot be set here; use ReorderSidecarTypes"))
	}

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		if entry.Id == "" {
			// A new type is created disabled. It classifies nothing until the
			// ordering transaction gives it a place in the sequence.
			entry.Id = uuid.NewString()
			entry.Order = 0
			cfg.DirectoryScanner.SidecarTypes = append(cfg.DirectoryScanner.SidecarTypes, entry)
		} else {
			index := appconfig.FindSidecarTypeIndexByID(cfg.DirectoryScanner, entry.Id)
			if index == -1 {
				// An unknown id is a mistake worth surfacing, not an invitation to
				// create an entry under an id the caller chose.
				return connectError(http.StatusNotFound, errors.New("no sidecar type with that id"))
			}
			entry.Order = cfg.DirectoryScanner.SidecarTypes[index].Order
			cfg.DirectoryScanner.SidecarTypes[index] = entry
		}

		// Compile the resulting table, not just the submitted entry. A bad
		// pattern only becomes visible on compilation, and validating the whole
		// table also catches the duplicate a partial check would miss.
		// Rejecting here means the error reaches whoever is editing, rather
		// than a scan log nobody is reading.
		if _, err := scanmodel.NewSidecarRegistry(cfg.DirectoryScanner.SidecarTypes); err != nil {
			return connectError(http.StatusBadRequest, err)
		}
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *DirectoryScannerServer) DeleteSidecarType(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceDeleteSidecarTypeRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)
	id := req.Msg.GetId()

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		index := appconfig.FindSidecarTypeIndexByID(cfg.DirectoryScanner, id)
		if index == -1 {
			return connectError(http.StatusNotFound, errors.New("no sidecar type with that id"))
		}
		sidecarTypes := cfg.DirectoryScanner.SidecarTypes
		cfg.DirectoryScanner.SidecarTypes = append(sidecarTypes[:index], sidecarTypes[index+1:]...)
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *DirectoryScannerServer) ReorderSidecarTypes(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceReorderSidecarTypesRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)
	requested := req.Msg.GetOrders()

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		sidecarTypes := cfg.DirectoryScanner.SidecarTypes

		// Every stored entry has to be accounted for, and nothing may be named
		// that does not exist. Both directions are checked before anything is
		// applied, so a rejected request leaves the stored order completely
		// untouched.
		var missing []string
		for _, entry := range sidecarTypes {
			if _, present := requested[entry.Id]; !present {
				missing = append(missing, fmt.Sprintf("%s (%s)", entry.Id, entry.Type))
			}
		}
		if len(missing) > 0 {
			return connectError(http.StatusBadRequest, fmt.Errorf("the order must name every sidecar type; missing: %s", strings.Join(missing, ", ")))
		}
		for id := range requested {
			if appconfig.FindSidecarTypeIndexByID(cfg.DirectoryScanner, id) == -1 {
				return connectError(http.StatusBadRequest, fmt.Errorf("no sidecar type with id %s", id))
			}
		}

		for i := range sidecarTypes {
			sidecarTypes[i].Order = requested[sidecarTypes[i].Id]
		}

		// The registry is the authority on what makes a coherent table,
		// duplicate orders included, so the result is run past it rather than
		// duplicating the rule here.
		if _, err := scanmodel.NewSidecarRegistry(sidecarTypes); err != nil {
			return connectError(http.StatusBadRequest, err)
		}
		cfg.DirectoryScanner.SidecarTypes = sidecarTypes
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *DirectoryScannerServer) ResetSidecarTypes(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceResetSidecarTypesRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	err := s.AppConfigStore.Mutate(ctx, func(cfg *appconfig.Config) error {
		cfg.DirectoryScanner.SidecarTypes = appconfig.DefaultSidecarTypes()
		return nil
	})
	if err != nil {
		return mutateConfigError(s.Logger, correlationID, err)
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}
