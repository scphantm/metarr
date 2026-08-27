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

// DirectoryScannerServer implements metarrv1connect.DirectoryScannerServiceHandler,
// ported directly from internal/server/handlers/directory_scanner.go — same
// Mongo reads and FireConfigUpdate call, only the transport changed.
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
	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}
	return connect.NewResponse(&metarrv1.DirectoryScannerServiceGetResponse{
		Config: directoryScannerConfigToProto(appConfig.DirectoryScanner),
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

	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	if req.Msg.ParallelCount != nil {
		appConfig.DirectoryScanner.ParallelCount = int(req.Msg.GetParallelCount())
	}

	if err := s.FireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		s.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue config update"))
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *DirectoryScannerServer) ListDirectories(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceListDirectoriesRequest],
) (*connect.Response[metarrv1.DirectoryScannerServiceListDirectoriesResponse], error) {
	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	dirs := make([]*metarrv1.ScanDirectory, 0, len(appConfig.DirectoryScanner.ScanDirectories))
	for _, dir := range appConfig.DirectoryScanner.ScanDirectories {
		dirs = append(dirs, scanDirectoryToProto(dir))
	}
	return connect.NewResponse(&metarrv1.DirectoryScannerServiceListDirectoriesResponse{Directories: dirs}), nil
}

func (s *DirectoryScannerServer) GetDirectory(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceGetDirectoryRequest],
) (*connect.Response[metarrv1.DirectoryScannerServiceGetDirectoryResponse], error) {
	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	index := appConfig.DirectoryScanner.FindScanDirectoryIndex(req.Msg.GetSlug())
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no scan directory with that slug"))
	}
	return connect.NewResponse(&metarrv1.DirectoryScannerServiceGetDirectoryResponse{
		Directory: scanDirectoryToProto(appConfig.DirectoryScanner.ScanDirectories[index]),
	}), nil
}

func (s *DirectoryScannerServer) UpsertDirectory(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceUpsertDirectoryRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	entry := scanDirectoryFromProto(req.Msg.GetDirectory())
	if entry.ScannerSlug == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("scanner_slug is required"))
	}
	// Reject an unscannable scan_type here rather than letting it fail later in
	// the scan listener, where nobody is waiting to see the error.
	if _, err := scanmodel.ParseDirectoryType(entry.ScanType); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	if index := appConfig.DirectoryScanner.FindScanDirectoryIndex(entry.ScannerSlug); index == -1 {
		appConfig.DirectoryScanner.ScanDirectories = append(appConfig.DirectoryScanner.ScanDirectories, entry)
	} else {
		appConfig.DirectoryScanner.ScanDirectories[index] = entry
	}

	if err := s.FireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		s.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue config update"))
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *DirectoryScannerServer) DeleteDirectory(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceDeleteDirectoryRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)
	slug := req.Msg.GetSlug()

	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	index := appConfig.DirectoryScanner.FindScanDirectoryIndex(slug)
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no scan directory with that slug"))
	}
	scanDirectories := appConfig.DirectoryScanner.ScanDirectories
	appConfig.DirectoryScanner.ScanDirectories = append(scanDirectories[:index], scanDirectories[index+1:]...)

	if err := s.FireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		s.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue config update"))
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *DirectoryScannerServer) ListSidecarTypes(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceListSidecarTypesRequest],
) (*connect.Response[metarrv1.DirectoryScannerServiceListSidecarTypesResponse], error) {
	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	types := make([]*metarrv1.SidecarTypeDefinition, 0, len(appConfig.DirectoryScanner.SidecarTypes))
	for _, def := range appConfig.DirectoryScanner.SidecarTypes {
		types = append(types, sidecarTypeDefinitionToProto(def))
	}
	return connect.NewResponse(&metarrv1.DirectoryScannerServiceListSidecarTypesResponse{Types: types}), nil
}

func (s *DirectoryScannerServer) GetSidecarType(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceGetSidecarTypeRequest],
) (*connect.Response[metarrv1.DirectoryScannerServiceGetSidecarTypeResponse], error) {
	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	index := appConfig.DirectoryScanner.FindSidecarTypeIndexByID(req.Msg.GetId())
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no sidecar type with that id"))
	}
	return connect.NewResponse(&metarrv1.DirectoryScannerServiceGetSidecarTypeResponse{
		Type: sidecarTypeDefinitionToProto(appConfig.DirectoryScanner.SidecarTypes[index]),
	}), nil
}

func (s *DirectoryScannerServer) UpsertSidecarType(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceUpsertSidecarTypeRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	entry := sidecarTypeDefinitionFromProto(req.Msg.GetType())
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

	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	if entry.ID == "" {
		// A new type is created disabled. It classifies nothing until the
		// ordering transaction gives it a place in the sequence.
		entry.ID = uuid.NewString()
		entry.Order = 0
		appConfig.DirectoryScanner.SidecarTypes = append(appConfig.DirectoryScanner.SidecarTypes, entry)
	} else {
		index := appConfig.DirectoryScanner.FindSidecarTypeIndexByID(entry.ID)
		if index == -1 {
			// An unknown id is a mistake worth surfacing, not an invitation to
			// create an entry under an id the caller chose.
			return nil, connectError(http.StatusNotFound, errors.New("no sidecar type with that id"))
		}
		entry.Order = appConfig.DirectoryScanner.SidecarTypes[index].Order
		appConfig.DirectoryScanner.SidecarTypes[index] = entry
	}

	// Compile the resulting table, not just the submitted entry. A bad pattern
	// only becomes visible on compilation, and validating the whole table also
	// catches the duplicate a partial check would miss. Rejecting here means the
	// error reaches whoever is editing, rather than a scan log nobody is reading.
	if _, err := scanmodel.NewSidecarRegistry(appConfig.DirectoryScanner.SidecarTypes); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	if err := s.FireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		s.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue config update"))
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *DirectoryScannerServer) DeleteSidecarType(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceDeleteSidecarTypeRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)
	id := req.Msg.GetId()

	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	index := appConfig.DirectoryScanner.FindSidecarTypeIndexByID(id)
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no sidecar type with that id"))
	}
	sidecarTypes := appConfig.DirectoryScanner.SidecarTypes
	appConfig.DirectoryScanner.SidecarTypes = append(sidecarTypes[:index], sidecarTypes[index+1:]...)

	if err := s.FireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		s.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue config update"))
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *DirectoryScannerServer) ReorderSidecarTypes(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceReorderSidecarTypesRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)
	requested := req.Msg.GetOrders()

	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}
	sidecarTypes := appConfig.DirectoryScanner.SidecarTypes

	// Every stored entry has to be accounted for, and nothing may be named that
	// does not exist. Both directions are checked before anything is applied, so
	// a rejected request leaves the stored order completely untouched.
	var missing []string
	for _, entry := range sidecarTypes {
		if _, present := requested[entry.ID]; !present {
			missing = append(missing, fmt.Sprintf("%s (%s)", entry.ID, entry.Type))
		}
	}
	if len(missing) > 0 {
		return nil, connectError(http.StatusBadRequest, fmt.Errorf("the order must name every sidecar type; missing: %s", strings.Join(missing, ", ")))
	}
	for id := range requested {
		if appConfig.DirectoryScanner.FindSidecarTypeIndexByID(id) == -1 {
			return nil, connectError(http.StatusBadRequest, fmt.Errorf("no sidecar type with id %s", id))
		}
	}

	for i := range sidecarTypes {
		sidecarTypes[i].Order = int(requested[sidecarTypes[i].ID])
	}

	// The registry is the authority on what makes a coherent table, duplicate
	// orders included, so the result is run past it rather than duplicating the
	// rule here.
	if _, err := scanmodel.NewSidecarRegistry(sidecarTypes); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}
	appConfig.DirectoryScanner.SidecarTypes = sidecarTypes

	if err := s.FireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		s.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue config update"))
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}

func (s *DirectoryScannerServer) ResetSidecarTypes(
	ctx context.Context,
	req *connect.Request[metarrv1.DirectoryScannerServiceResetSidecarTypesRequest],
) (*connect.Response[metarrv1.AcceptedResponse], error) {
	correlationID := correlation.FromContext(ctx)

	appConfig, err := s.AppConfigRepo.Get(ctx)
	if err != nil {
		s.Logger.Error("failed to fetch app config", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to fetch config"))
	}

	appConfig.DirectoryScanner.SidecarTypes = appconfig.DefaultSidecarTypes()

	if err := s.FireConfigUpdate(ctx, correlationID, *appConfig); err != nil {
		s.Logger.Error("failed to fire system_config_update event", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to queue config update"))
	}

	return connect.NewResponse(acceptedResponse(correlationID)), nil
}
