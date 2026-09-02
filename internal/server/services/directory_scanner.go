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
	"Metarr/internal/shared/scanmodel"
)

// DirectoryScannerServer implements
// metarrv1connect.DirectoryScannerServiceHandler: the Directory Scanner
// settings on AIP standard methods (docs/adr/0010) — a scalar section, a
// slug-addressed scan-directory collection, and a minted-id sidecar-type
// collection. Reads come from live config; every write goes through
// AppConfigStore.MutateSync, which persists under the store lock and
// propagates in-process before returning (docs/adr/0002).
type DirectoryScannerServer struct {
	*handlers.Handlers
}

// DirectoryScannerAuthPolicies is this service's method-name -> policy map,
// passed to httpserver.NewConnectAuthInterceptor when mounting it. Every
// route is GroupConfig; the reads are read-only.
var DirectoryScannerAuthPolicies = map[string]httpserver.RPCPolicy{
	"GetDirectoryScannerConfig":    {Group: auth.GroupConfig, ReadOnly: true},
	"UpdateDirectoryScannerConfig": {Group: auth.GroupConfig},
	"CreateScanDirectory":          {Group: auth.GroupConfig},
	"GetScanDirectory":             {Group: auth.GroupConfig, ReadOnly: true},
	"ListScanDirectories":          {Group: auth.GroupConfig, ReadOnly: true},
	"UpdateScanDirectory":          {Group: auth.GroupConfig},
	"DeleteScanDirectory":          {Group: auth.GroupConfig},
	"CreateSidecarType":            {Group: auth.GroupConfig},
	"GetSidecarType":               {Group: auth.GroupConfig, ReadOnly: true},
	"ListSidecarTypes":             {Group: auth.GroupConfig, ReadOnly: true},
	"UpdateSidecarType":            {Group: auth.GroupConfig},
	"DeleteSidecarType":            {Group: auth.GroupConfig},
	"ReorderSidecarTypes":          {Group: auth.GroupConfig},
	"ResetSidecarTypes":            {Group: auth.GroupConfig},
}

// scanDirectoryOrderFields maps the order_by paths ListScanDirectories
// accepts to their comparators; any other path is InvalidArgument.
var scanDirectoryOrderFields = map[string]func(a, b *metarrv1.ScanDirectory) int{
	"scanner_slug": func(a, b *metarrv1.ScanDirectory) int {
		return strings.Compare(a.GetScannerSlug(), b.GetScannerSlug())
	},
	"scan_type": func(a, b *metarrv1.ScanDirectory) int {
		return strings.Compare(a.GetScanType(), b.GetScanType())
	},
}

// sidecarTypeOrderFields maps the order_by paths ListSidecarTypes accepts to
// their comparators; any other path is InvalidArgument.
var sidecarTypeOrderFields = map[string]func(a, b *appconfig.SidecarTypeDefinition) int{
	"id": func(a, b *appconfig.SidecarTypeDefinition) int {
		return strings.Compare(a.GetId(), b.GetId())
	},
	"type": func(a, b *appconfig.SidecarTypeDefinition) int {
		return strings.Compare(a.GetType(), b.GetType())
	},
	"order": func(a, b *appconfig.SidecarTypeDefinition) int {
		switch {
		case a.GetOrder() < b.GetOrder():
			return -1
		case a.GetOrder() > b.GetOrder():
			return 1
		default:
			return 0
		}
	},
}

// --- Scalar section -------------------------------------------------------

func (s *DirectoryScannerServer) GetDirectoryScannerConfig(
	ctx context.Context,
	req *connect.Request[metarrv1.GetDirectoryScannerConfigRequest],
) (*connect.Response[metarrv1.GetDirectoryScannerConfigResponse], error) {
	return connect.NewResponse(&metarrv1.GetDirectoryScannerConfigResponse{
		Config: cloneMsg(appconfig.Get().DirectoryScanner),
	}), nil
}

// UpdateDirectoryScannerConfig is an AIP-134 partial update of the scalar
// section: update_mask may name only parallel_count. An empty mask, or one
// naming anything else, is InvalidArgument — scan_directories and
// sidecar_types are edited through their own methods. The merged section is
// validated as a whole and the write is synchronous (docs/adr/0002).
func (s *DirectoryScannerServer) UpdateDirectoryScannerConfig(
	ctx context.Context,
	req *connect.Request[metarrv1.UpdateDirectoryScannerConfigRequest],
) (*connect.Response[metarrv1.DirectoryScannerConfig], error) {
	correlationID := correlation.FromContext(ctx)

	patch := req.Msg.GetConfig()
	if patch == nil {
		return nil, connectError(http.StatusBadRequest, errors.New("config is required"))
	}
	for _, path := range req.Msg.GetUpdateMask().GetPaths() {
		if path != "parallel_count" {
			return nil, connectError(http.StatusBadRequest,
				fmt.Errorf("%w: the scalar section update_mask may name only parallel_count", errUnknownPath))
		}
	}

	var stored *metarrv1.DirectoryScannerConfig
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		merged := cloneMsg(cfg.DirectoryScanner)
		if merged == nil {
			merged = &metarrv1.DirectoryScannerConfig{}
		}
		if err := applyUpdateMask(merged, patch, req.Msg.GetUpdateMask()); err != nil {
			return err
		}
		if merged.GetParallelCount() <= 0 {
			return connectError(http.StatusBadRequest, errors.New("parallel_count must be greater than zero"))
		}
		cfg.DirectoryScanner = merged
		stored = cloneMsg(merged)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(stored), nil
}

// --- Scan directories: slug-addressed collection ------------------------

func (s *DirectoryScannerServer) CreateScanDirectory(
	ctx context.Context,
	req *connect.Request[metarrv1.CreateScanDirectoryRequest],
) (*connect.Response[metarrv1.ScanDirectory], error) {
	correlationID := correlation.FromContext(ctx)

	slug := req.Msg.GetScanDirectoryId()
	entry := cloneOrNewScanDirectory(req.Msg.GetScanDirectory())
	// AIP-133: a slug in the body must match scan_directory_id or be empty.
	if bodySlug := entry.GetScannerSlug(); bodySlug != "" && bodySlug != slug {
		return nil, connectError(http.StatusBadRequest,
			fmt.Errorf("scan_directory.scanner_slug %q does not match scan_directory_id %q", bodySlug, slug))
	}
	entry.ScannerSlug = slug

	var stored *metarrv1.ScanDirectory
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		created, err := appendNewScanDirectory(cfg, entry)
		stored = created
		return err
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(stored), nil
}

func (s *DirectoryScannerServer) GetScanDirectory(
	ctx context.Context,
	req *connect.Request[metarrv1.GetScanDirectoryRequest],
) (*connect.Response[metarrv1.ScanDirectory], error) {
	cfg := appconfig.Get()
	index := appconfig.FindScanDirectoryIndex(cfg.DirectoryScanner, req.Msg.GetSlug())
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no scan directory with that slug"))
	}
	return connect.NewResponse(cloneMsg(cfg.DirectoryScanner.ScanDirectories[index])), nil
}

func (s *DirectoryScannerServer) ListScanDirectories(
	ctx context.Context,
	req *connect.Request[metarrv1.ListScanDirectoriesRequest],
) (*connect.Response[metarrv1.ListScanDirectoriesResponse], error) {
	if err := parseListFilter(req.Msg.GetFilter()); err != nil {
		return nil, aipConnectError(err)
	}

	cfg := appconfig.Get()
	dirs := make([]*metarrv1.ScanDirectory, 0, len(cfg.DirectoryScanner.ScanDirectories))
	for _, dir := range cfg.DirectoryScanner.ScanDirectories {
		dirs = append(dirs, cloneMsg(dir))
	}

	if err := orderBySlice(dirs, req.Msg.GetOrderBy(), scanDirectoryOrderFields); err != nil {
		return nil, aipConnectError(err)
	}

	page, nextPageToken, err := paginateSlice(dirs, req.Msg.GetPageSize(), req.Msg.GetPageToken())
	if err != nil {
		return nil, aipConnectError(err)
	}
	return connect.NewResponse(&metarrv1.ListScanDirectoriesResponse{
		ScanDirectories: page,
		NextPageToken:   nextPageToken,
	}), nil
}

func (s *DirectoryScannerServer) UpdateScanDirectory(
	ctx context.Context,
	req *connect.Request[metarrv1.UpdateScanDirectoryRequest],
) (*connect.Response[metarrv1.ScanDirectory], error) {
	correlationID := correlation.FromContext(ctx)

	patch := req.Msg.GetScanDirectory()
	if patch == nil {
		return nil, connectError(http.StatusBadRequest, errors.New("scan_directory is required"))
	}
	slug := patch.GetScannerSlug()
	if slug == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("scan_directory.scanner_slug is required"))
	}

	var stored *metarrv1.ScanDirectory
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		index := appconfig.FindScanDirectoryIndex(cfg.DirectoryScanner, slug)
		if index == -1 {
			// allow_missing:true upgrades an Update on an unknown slug to a
			// Create — the mask is ignored and the whole resource is
			// validated as a Create (docs/adr/0010).
			if !req.Msg.GetAllowMissing() {
				return connectError(http.StatusNotFound, errors.New("no scan directory with that slug"))
			}
			created, err := appendNewScanDirectory(cfg, cloneMsg(patch))
			stored = created
			return err
		}

		merged := cloneMsg(cfg.DirectoryScanner.ScanDirectories[index])
		if err := applyUpdateMask(merged, patch, req.Msg.GetUpdateMask()); err != nil {
			return err
		}
		// The mask cannot move the slug — it is the addressing key.
		merged.ScannerSlug = slug
		if err := validateScanDirectory(merged); err != nil {
			return connectError(http.StatusBadRequest, err)
		}
		cfg.DirectoryScanner.ScanDirectories[index] = merged
		stored = cloneMsg(merged)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(stored), nil
}

func (s *DirectoryScannerServer) DeleteScanDirectory(
	ctx context.Context,
	req *connect.Request[metarrv1.DeleteScanDirectoryRequest],
) (*connect.Response[emptypb.Empty], error) {
	correlationID := correlation.FromContext(ctx)

	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		index := appconfig.FindScanDirectoryIndex(cfg.DirectoryScanner, req.Msg.GetSlug())
		if index == -1 {
			return connectError(http.StatusNotFound, errors.New("no scan directory with that slug"))
		}
		dirs := cfg.DirectoryScanner.ScanDirectories
		cfg.DirectoryScanner.ScanDirectories = append(dirs[:index], dirs[index+1:]...)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

// cloneOrNewScanDirectory returns a detached copy of the request's directory,
// or a fresh one when the request carried none.
func cloneOrNewScanDirectory(entry *metarrv1.ScanDirectory) *metarrv1.ScanDirectory {
	if entry == nil {
		return &metarrv1.ScanDirectory{}
	}
	return cloneMsg(entry)
}

// appendNewScanDirectory validates entry as a whole resource and adds it to
// cfg; an existing scanner_slug is AlreadyExists. It is the shared body of
// CreateScanDirectory and the allow_missing branch of UpdateScanDirectory.
// It returns the stored clone; the closures that call it run under the store
// lock.
func appendNewScanDirectory(cfg *appconfig.Config, entry *metarrv1.ScanDirectory) (*metarrv1.ScanDirectory, error) {
	if err := validateScanDirectory(entry); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}
	if appconfig.FindScanDirectoryIndex(cfg.DirectoryScanner, entry.GetScannerSlug()) != -1 {
		return nil, connectError(http.StatusConflict,
			fmt.Errorf("a scan directory with slug %q already exists", entry.GetScannerSlug()))
	}
	cfg.DirectoryScanner.ScanDirectories = append(cfg.DirectoryScanner.ScanDirectories, entry)
	return cloneMsg(entry), nil
}

// validateScanDirectory is the whole-resource check a Create (and an
// allow_missing Update that creates) runs before the store writes: a slug is
// required, and an unscannable scan_type is rejected here rather than left to
// fail later in the scan listener where nobody is waiting to see the error.
func validateScanDirectory(entry *metarrv1.ScanDirectory) error {
	if entry.GetScannerSlug() == "" {
		return errors.New("scanner_slug is required")
	}
	if _, err := scanmodel.ParseDirectoryType(entry.GetScanType()); err != nil {
		return err
	}
	return nil
}

// --- Sidecar types: minted-id collection -------------------------------

func (s *DirectoryScannerServer) CreateSidecarType(
	ctx context.Context,
	req *connect.Request[metarrv1.CreateSidecarTypeRequest],
) (*connect.Response[appconfig.SidecarTypeDefinition], error) {
	correlationID := correlation.FromContext(ctx)

	entry := req.Msg.GetSidecarType()
	if entry == nil {
		entry = &appconfig.SidecarTypeDefinition{}
	} else {
		entry = cloneMsg(entry)
	}
	if entry.GetId() != "" {
		return nil, connectError(http.StatusBadRequest,
			errors.New("id is server-minted and must not be set on Create"))
	}
	// Rejected rather than ignored: silently dropping an order someone took
	// the trouble to send would leave them believing they had placed the
	// entry in the evaluation sequence.
	if entry.GetOrder() != 0 {
		return nil, connectError(http.StatusBadRequest,
			errors.New("order cannot be set here; a new type is created disabled — use ReorderSidecarTypes"))
	}
	if err := validateSidecarType(entry); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	var stored *appconfig.SidecarTypeDefinition
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		// A new type is created disabled. It classifies nothing until the
		// ordering transaction gives it a place in the sequence.
		entry.Id = uuid.NewString()
		entry.Order = 0
		cfg.DirectoryScanner.SidecarTypes = append(cfg.DirectoryScanner.SidecarTypes, entry)
		if err := compileSidecarRegistry(cfg); err != nil {
			return err
		}
		stored = cloneMsg(entry)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(stored), nil
}

func (s *DirectoryScannerServer) GetSidecarType(
	ctx context.Context,
	req *connect.Request[metarrv1.GetSidecarTypeRequest],
) (*connect.Response[appconfig.SidecarTypeDefinition], error) {
	cfg := appconfig.Get()
	index := appconfig.FindSidecarTypeIndexByID(cfg.DirectoryScanner, req.Msg.GetId())
	if index == -1 {
		return nil, connectError(http.StatusNotFound, errors.New("no sidecar type with that id"))
	}
	return connect.NewResponse(cloneMsg(cfg.DirectoryScanner.SidecarTypes[index])), nil
}

func (s *DirectoryScannerServer) ListSidecarTypes(
	ctx context.Context,
	req *connect.Request[metarrv1.ListSidecarTypesRequest],
) (*connect.Response[metarrv1.ListSidecarTypesResponse], error) {
	if err := parseListFilter(req.Msg.GetFilter()); err != nil {
		return nil, aipConnectError(err)
	}

	cfg := appconfig.Get()
	types := make([]*appconfig.SidecarTypeDefinition, 0, len(cfg.DirectoryScanner.SidecarTypes))
	for _, def := range cfg.DirectoryScanner.SidecarTypes {
		types = append(types, cloneMsg(def))
	}

	if err := orderBySlice(types, req.Msg.GetOrderBy(), sidecarTypeOrderFields); err != nil {
		return nil, aipConnectError(err)
	}

	page, nextPageToken, err := paginateSlice(types, req.Msg.GetPageSize(), req.Msg.GetPageToken())
	if err != nil {
		return nil, aipConnectError(err)
	}
	return connect.NewResponse(&metarrv1.ListSidecarTypesResponse{
		SidecarTypes:  page,
		NextPageToken: nextPageToken,
	}), nil
}

// UpdateSidecarType is an AIP-134 partial update matched by minted id. There
// is no allow_missing — an unknown id is NotFound, never a create. The order
// field cannot be moved by the mask (use ReorderSidecarTypes); the resulting
// table is compiled and a table that fails to compile is InvalidArgument.
func (s *DirectoryScannerServer) UpdateSidecarType(
	ctx context.Context,
	req *connect.Request[metarrv1.UpdateSidecarTypeRequest],
) (*connect.Response[appconfig.SidecarTypeDefinition], error) {
	correlationID := correlation.FromContext(ctx)

	patch := req.Msg.GetSidecarType()
	if patch == nil {
		return nil, connectError(http.StatusBadRequest, errors.New("sidecar_type is required"))
	}
	id := patch.GetId()
	if id == "" {
		return nil, connectError(http.StatusBadRequest, errors.New("sidecar_type.id is required"))
	}
	for _, path := range req.Msg.GetUpdateMask().GetPaths() {
		if path == "order" || path == "id" {
			return nil, connectError(http.StatusBadRequest,
				fmt.Errorf("%w: %s cannot be changed here", errUnknownPath, path))
		}
	}

	var stored *appconfig.SidecarTypeDefinition
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		index := appconfig.FindSidecarTypeIndexByID(cfg.DirectoryScanner, id)
		if index == -1 {
			return connectError(http.StatusNotFound, errors.New("no sidecar type with that id"))
		}

		merged := cloneMsg(cfg.DirectoryScanner.SidecarTypes[index])
		if err := applyUpdateMask(merged, patch, req.Msg.GetUpdateMask()); err != nil {
			return err
		}
		// id and order are not the mask's to move: id is the addressing key,
		// order belongs to the reorder transaction.
		merged.Id = id
		merged.Order = cfg.DirectoryScanner.SidecarTypes[index].GetOrder()
		if err := validateSidecarType(merged); err != nil {
			return connectError(http.StatusBadRequest, err)
		}
		cfg.DirectoryScanner.SidecarTypes[index] = merged
		if err := compileSidecarRegistry(cfg); err != nil {
			return err
		}
		stored = cloneMsg(merged)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(stored), nil
}

func (s *DirectoryScannerServer) DeleteSidecarType(
	ctx context.Context,
	req *connect.Request[metarrv1.DeleteSidecarTypeRequest],
) (*connect.Response[emptypb.Empty], error) {
	correlationID := correlation.FromContext(ctx)

	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		index := appconfig.FindSidecarTypeIndexByID(cfg.DirectoryScanner, req.Msg.GetId())
		if index == -1 {
			return connectError(http.StatusNotFound, errors.New("no sidecar type with that id"))
		}
		types := cfg.DirectoryScanner.SidecarTypes
		cfg.DirectoryScanner.SidecarTypes = append(types[:index], types[index+1:]...)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *DirectoryScannerServer) ReorderSidecarTypes(
	ctx context.Context,
	req *connect.Request[metarrv1.ReorderSidecarTypesRequest],
) (*connect.Response[metarrv1.ReorderSidecarTypesResponse], error) {
	correlationID := correlation.FromContext(ctx)
	requested := req.Msg.GetOrders()

	var stored []*appconfig.SidecarTypeDefinition
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		types := cfg.DirectoryScanner.SidecarTypes

		// Every stored entry has to be accounted for, and nothing may be
		// named that does not exist. Both directions are checked before
		// anything is applied, so a rejected request leaves the stored order
		// completely untouched.
		var missing []string
		for _, entry := range types {
			if _, present := requested[entry.GetId()]; !present {
				missing = append(missing, fmt.Sprintf("%s (%s)", entry.GetId(), entry.GetType()))
			}
		}
		if len(missing) > 0 {
			return connectError(http.StatusBadRequest,
				fmt.Errorf("the order must name every sidecar type; missing: %s", strings.Join(missing, ", ")))
		}
		for id := range requested {
			if appconfig.FindSidecarTypeIndexByID(cfg.DirectoryScanner, id) == -1 {
				return connectError(http.StatusBadRequest, fmt.Errorf("no sidecar type with id %s", id))
			}
		}

		for i := range types {
			types[i].Order = requested[types[i].GetId()]
		}
		if err := compileSidecarRegistry(cfg); err != nil {
			return err
		}
		stored = cloneSidecarTypes(types)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(&metarrv1.ReorderSidecarTypesResponse{SidecarTypes: stored}), nil
}

func (s *DirectoryScannerServer) ResetSidecarTypes(
	ctx context.Context,
	req *connect.Request[metarrv1.ResetSidecarTypesRequest],
) (*connect.Response[metarrv1.ResetSidecarTypesResponse], error) {
	correlationID := correlation.FromContext(ctx)

	var stored []*appconfig.SidecarTypeDefinition
	err := s.AppConfigStore.MutateSync(ctx, func(cfg *appconfig.Config) error {
		cfg.DirectoryScanner.SidecarTypes = appconfig.DefaultSidecarTypes()
		stored = cloneSidecarTypes(cfg.DirectoryScanner.SidecarTypes)
		return nil
	})
	if err != nil {
		return nil, mutateConfigErr(s.Logger, correlationID, err)
	}
	return connect.NewResponse(&metarrv1.ResetSidecarTypesResponse{SidecarTypes: stored}), nil
}

// validateSidecarType is the whole-resource check a Create and an Update run
// before the store writes: a type name is required and the category must be
// one of the known sidecar categories.
func validateSidecarType(entry *appconfig.SidecarTypeDefinition) error {
	if entry.GetType() == "" {
		return errors.New("type is required")
	}
	if _, err := scanmodel.ParseSidecarCategory(entry.GetCategory()); err != nil {
		return err
	}
	return nil
}

// compileSidecarRegistry compiles the whole sidecar table, not just the
// submitted entry: a bad pattern only becomes visible on compilation, and
// validating the whole table also catches the duplicate a partial check
// would miss. Rejecting here means the error reaches whoever is editing,
// rather than a scan log nobody is reading. Runs under the store lock.
func compileSidecarRegistry(cfg *appconfig.Config) error {
	if _, err := scanmodel.NewSidecarRegistry(cfg.DirectoryScanner.SidecarTypes); err != nil {
		return connectError(http.StatusBadRequest, err)
	}
	return nil
}

func cloneSidecarTypes(types []*appconfig.SidecarTypeDefinition) []*appconfig.SidecarTypeDefinition {
	out := make([]*appconfig.SidecarTypeDefinition, 0, len(types))
	for _, t := range types {
		out = append(out, cloneMsg(t))
	}
	return out
}
