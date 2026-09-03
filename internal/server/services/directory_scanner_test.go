package services

import (
	"context"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/appconfigstore"
	"Metarr/internal/server/handlers"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

func newTestDirectoryScannerServer(seed *appconfig.Config) (*DirectoryScannerServer, *fakeConfigBackend) {
	backend := &fakeConfigBackend{cfg: seed}
	store := appconfigstore.New(backend, backend)
	return &DirectoryScannerServer{Handlers: &handlers.Handlers{
		AppConfigStore: store,
		Logger:         slog.Default(),
	}}, backend
}

func seededScanDirectory(slug string) *appconfig.ScanDirectory {
	return &appconfig.ScanDirectory{
		ScannerSlug: slug,
		ScanType:    "movie",
		Directory:   "/media/" + slug,
	}
}

func seededSidecarType(id, typeName string, order int32) *appconfig.SidecarTypeDefinition {
	return &appconfig.SidecarTypeDefinition{
		Id:       id,
		Type:     typeName,
		Category: "image",
		Order:    order,
		Patterns: []string{"(?i)^" + typeName + "$"},
	}
}

func dsConfig(dirs []*appconfig.ScanDirectory, types []*appconfig.SidecarTypeDefinition) *appconfig.Config {
	return &appconfig.Config{
		DirectoryScanner: &appconfig.DirectoryScannerConfig{
			ParallelCount:   4,
			ScanDirectories: dirs,
			SidecarTypes:    types,
		},
	}
}

// --- Scalar section -------------------------------------------------------

func TestDirectoryScannerScalar_UpdatesParallelCountViaMask(t *testing.T) {
	server, backend := newTestDirectoryScannerServer(dsConfig(nil, nil))

	ctx := correlation.WithID(context.Background(), "corr-1")
	resp, err := server.UpdateDirectoryScannerConfig(ctx, connect.NewRequest(&metarrv1.UpdateDirectoryScannerConfigRequest{
		Config:     &metarrv1.DirectoryScannerConfig{ParallelCount: 9},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"parallel_count"}},
	}))
	if err != nil {
		t.Fatalf("UpdateDirectoryScannerConfig: %v", err)
	}
	if resp.Msg.GetParallelCount() != 9 {
		t.Errorf("response parallel_count = %d, want 9", resp.Msg.GetParallelCount())
	}
	if got := backend.cfg.GetDirectoryScanner().GetParallelCount(); got != 9 {
		t.Errorf("persisted parallel_count = %d, want 9", got)
	}
	if len(backend.fired) != 0 {
		t.Fatalf("a synchronous write fired %d events, want 0", len(backend.fired))
	}
}

func TestDirectoryScannerScalar_RejectsEmptyAndForeignMask(t *testing.T) {
	server, _ := newTestDirectoryScannerServer(dsConfig(nil, nil))

	for name, mask := range map[string]*fieldmaskpb.FieldMask{
		"empty":        {},
		"foreign path": {Paths: []string{"scan_directories"}},
		"unknown path": {Paths: []string{"nope"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := server.UpdateDirectoryScannerConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateDirectoryScannerConfigRequest{
				Config:     &metarrv1.DirectoryScannerConfig{ParallelCount: 2},
				UpdateMask: mask,
			}))
			if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
				t.Fatalf("code = %v, want InvalidArgument", got)
			}
		})
	}
}

func TestDirectoryScannerScalar_RejectsNonPositiveParallelCount(t *testing.T) {
	server, _ := newTestDirectoryScannerServer(dsConfig(nil, nil))

	_, err := server.UpdateDirectoryScannerConfig(context.Background(), connect.NewRequest(&metarrv1.UpdateDirectoryScannerConfigRequest{
		Config:     &metarrv1.DirectoryScannerConfig{ParallelCount: 0},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"parallel_count"}},
	}))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", got)
	}
}

// --- Scan directories ---------------------------------------------------

func createDirReq(id string, dir *metarrv1.ScanDirectory) *connect.Request[metarrv1.CreateScanDirectoryRequest] {
	return connect.NewRequest(&metarrv1.CreateScanDirectoryRequest{ScanDirectoryId: id, ScanDirectory: dir})
}

func TestScanDirectoryCreate_AppendsAndReturnsStored(t *testing.T) {
	server, backend := newTestDirectoryScannerServer(dsConfig(nil, nil))

	resp, err := server.CreateScanDirectory(context.Background(), createDirReq("movies-4k", &metarrv1.ScanDirectory{
		ScanType:  "movie",
		Directory: "/media/movies-4k",
	}))
	if err != nil {
		t.Fatalf("CreateScanDirectory: %v", err)
	}
	if resp.Msg.GetScannerSlug() != "movies-4k" || resp.Msg.GetDirectory() != "/media/movies-4k" {
		t.Errorf("response = %+v", resp.Msg)
	}
	stored := backend.cfg.GetDirectoryScanner().GetScanDirectories()
	if len(stored) != 1 || stored[0].GetScannerSlug() != "movies-4k" {
		t.Fatalf("persisted = %+v", stored)
	}
}

func TestScanDirectoryCreate_ErrorMatrix(t *testing.T) {
	server, _ := newTestDirectoryScannerServer(dsConfig(
		[]*appconfig.ScanDirectory{seededScanDirectory("dup")}, nil))

	cases := map[string]struct {
		id   string
		dir  *metarrv1.ScanDirectory
		code connect.Code
	}{
		"existing slug":      {"dup", &metarrv1.ScanDirectory{ScanType: "movie"}, connect.CodeAlreadyExists},
		"slug/body mismatch": {"a", &metarrv1.ScanDirectory{ScannerSlug: "b", ScanType: "movie"}, connect.CodeInvalidArgument},
		"empty slug":         {"", &metarrv1.ScanDirectory{ScanType: "movie"}, connect.CodeInvalidArgument},
		"bad scan_type":      {"c", &metarrv1.ScanDirectory{ScanType: "nonsense"}, connect.CodeInvalidArgument},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := server.CreateScanDirectory(context.Background(), createDirReq(tc.id, tc.dir))
			if got := connect.CodeOf(err); got != tc.code {
				t.Fatalf("code = %v, want %v", got, tc.code)
			}
		})
	}
}

func TestScanDirectoryGet_FoundAndNotFound(t *testing.T) {
	withLiveConfig(t, dsConfig([]*appconfig.ScanDirectory{seededScanDirectory("known")}, nil))
	server := &DirectoryScannerServer{Handlers: &handlers.Handlers{}}

	resp, err := server.GetScanDirectory(context.Background(), connect.NewRequest(&metarrv1.GetScanDirectoryRequest{Slug: "known"}))
	if err != nil {
		t.Fatalf("GetScanDirectory: %v", err)
	}
	if resp.Msg.GetDirectory() != "/media/known" {
		t.Errorf("directory = %q", resp.Msg.GetDirectory())
	}

	_, err = server.GetScanDirectory(context.Background(), connect.NewRequest(&metarrv1.GetScanDirectoryRequest{Slug: "nope"}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestScanDirectoryUpdate_PartialMaskLeavesSiblingsUntouched(t *testing.T) {
	server, backend := newTestDirectoryScannerServer(dsConfig(
		[]*appconfig.ScanDirectory{seededScanDirectory("main")}, nil))

	_, err := server.UpdateScanDirectory(context.Background(), connect.NewRequest(&metarrv1.UpdateScanDirectoryRequest{
		ScanDirectory: &metarrv1.ScanDirectory{ScannerSlug: "main", Directory: "/new/path", ScanType: "tv"},
		UpdateMask:    &fieldmaskpb.FieldMask{Paths: []string{"directory"}},
	}))
	if err != nil {
		t.Fatalf("UpdateScanDirectory: %v", err)
	}
	got := backend.cfg.GetDirectoryScanner().GetScanDirectories()[0]
	if got.GetDirectory() != "/new/path" {
		t.Errorf("directory = %q, want /new/path", got.GetDirectory())
	}
	if got.GetScanType() != "movie" {
		t.Errorf("scan_type = %q, want the seeded movie — an unmasked field moved", got.GetScanType())
	}
}

func TestScanDirectoryUpdate_MaskAndSlugErrors(t *testing.T) {
	server, _ := newTestDirectoryScannerServer(dsConfig(
		[]*appconfig.ScanDirectory{seededScanDirectory("main")}, nil))

	t.Run("empty mask", func(t *testing.T) {
		_, err := server.UpdateScanDirectory(context.Background(), connect.NewRequest(&metarrv1.UpdateScanDirectoryRequest{
			ScanDirectory: &metarrv1.ScanDirectory{ScannerSlug: "main", Directory: "/x"},
		}))
		if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
			t.Fatalf("code = %v, want InvalidArgument", got)
		}
	})
	t.Run("unknown path", func(t *testing.T) {
		_, err := server.UpdateScanDirectory(context.Background(), connect.NewRequest(&metarrv1.UpdateScanDirectoryRequest{
			ScanDirectory: &metarrv1.ScanDirectory{ScannerSlug: "main"},
			UpdateMask:    &fieldmaskpb.FieldMask{Paths: []string{"nope"}},
		}))
		if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
			t.Fatalf("code = %v, want InvalidArgument", got)
		}
	})
	t.Run("unknown slug without allow_missing", func(t *testing.T) {
		_, err := server.UpdateScanDirectory(context.Background(), connect.NewRequest(&metarrv1.UpdateScanDirectoryRequest{
			ScanDirectory: &metarrv1.ScanDirectory{ScannerSlug: "ghost", Directory: "/x"},
			UpdateMask:    &fieldmaskpb.FieldMask{Paths: []string{"directory"}},
		}))
		if got := connect.CodeOf(err); got != connect.CodeNotFound {
			t.Fatalf("code = %v, want NotFound", got)
		}
	})
}

func TestScanDirectoryUpdate_AllowMissingCreates(t *testing.T) {
	server, backend := newTestDirectoryScannerServer(dsConfig(nil, nil))

	resp, err := server.UpdateScanDirectory(context.Background(), connect.NewRequest(&metarrv1.UpdateScanDirectoryRequest{
		ScanDirectory: &metarrv1.ScanDirectory{ScannerSlug: "fresh", ScanType: "movie", Directory: "/media/fresh"},
		UpdateMask:    &fieldmaskpb.FieldMask{Paths: []string{"bogus"}},
		AllowMissing:  true,
	}))
	if err != nil {
		t.Fatalf("UpdateScanDirectory allow_missing: %v", err)
	}
	if resp.Msg.GetScannerSlug() != "fresh" {
		t.Errorf("response = %+v", resp.Msg)
	}
	if got := backend.cfg.GetDirectoryScanner().GetScanDirectories(); len(got) != 1 {
		t.Fatalf("persisted = %+v, want one entry", got)
	}
}

func TestScanDirectoryDelete_RemovesAndNotFound(t *testing.T) {
	server, backend := newTestDirectoryScannerServer(dsConfig(
		[]*appconfig.ScanDirectory{seededScanDirectory("a"), seededScanDirectory("b")}, nil))

	if _, err := server.DeleteScanDirectory(context.Background(), connect.NewRequest(&metarrv1.DeleteScanDirectoryRequest{Slug: "a"})); err != nil {
		t.Fatalf("DeleteScanDirectory: %v", err)
	}
	if got := backend.cfg.GetDirectoryScanner().GetScanDirectories(); len(got) != 1 || got[0].GetScannerSlug() != "b" {
		t.Fatalf("after delete = %+v, want only b", got)
	}
	_, err := server.DeleteScanDirectory(context.Background(), connect.NewRequest(&metarrv1.DeleteScanDirectoryRequest{Slug: "a"}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestScanDirectoryList_PaginatesOrdersAndRejectsFilter(t *testing.T) {
	withLiveConfig(t, dsConfig([]*appconfig.ScanDirectory{
		seededScanDirectory("c"), seededScanDirectory("a"), seededScanDirectory("b"),
	}, nil))
	server := &DirectoryScannerServer{Handlers: &handlers.Handlers{}}

	first, err := server.ListScanDirectories(context.Background(), connect.NewRequest(&metarrv1.ListScanDirectoriesRequest{
		PageSize: 2, OrderBy: "scanner_slug",
	}))
	if err != nil {
		t.Fatalf("ListScanDirectories: %v", err)
	}
	if len(first.Msg.GetScanDirectories()) != 2 || first.Msg.GetNextPageToken() == "" {
		t.Fatalf("page 1 = %d dirs, token %q", len(first.Msg.GetScanDirectories()), first.Msg.GetNextPageToken())
	}
	if first.Msg.GetScanDirectories()[0].GetScannerSlug() != "a" {
		t.Errorf("order = %q, want a first", first.Msg.GetScanDirectories()[0].GetScannerSlug())
	}
	second, err := server.ListScanDirectories(context.Background(), connect.NewRequest(&metarrv1.ListScanDirectoriesRequest{
		PageSize: 2, OrderBy: "scanner_slug", PageToken: first.Msg.GetNextPageToken(),
	}))
	if err != nil {
		t.Fatalf("ListScanDirectories page 2: %v", err)
	}
	if len(second.Msg.GetScanDirectories()) != 1 || second.Msg.GetNextPageToken() != "" {
		t.Fatalf("page 2 = %d dirs, token %q", len(second.Msg.GetScanDirectories()), second.Msg.GetNextPageToken())
	}

	_, err = server.ListScanDirectories(context.Background(), connect.NewRequest(&metarrv1.ListScanDirectoriesRequest{
		Filter: `directory = "/x"`,
	}))
	if got := connect.CodeOf(err); got != connect.CodeUnimplemented {
		t.Fatalf("code = %v, want Unimplemented", got)
	}
}

// --- Sidecar types ----------------------------------------------------

func TestSidecarTypeCreate_MintsIDAndReturnsStored(t *testing.T) {
	server, backend := newTestDirectoryScannerServer(dsConfig(nil, nil))

	resp, err := server.CreateSidecarType(context.Background(), connect.NewRequest(&metarrv1.CreateSidecarTypeRequest{
		SidecarType: &appconfig.SidecarTypeDefinition{Type: "storyboard", Category: "image", Patterns: []string{"(?i)^storyboard$"}},
	}))
	if err != nil {
		t.Fatalf("CreateSidecarType: %v", err)
	}
	if resp.Msg.GetId() == "" {
		t.Fatal("Create returned an entry with no minted id")
	}
	if resp.Msg.GetOrder() != 0 {
		t.Errorf("order = %d, want 0 — a new type is created disabled", resp.Msg.GetOrder())
	}
	stored := backend.cfg.GetDirectoryScanner().GetSidecarTypes()
	if len(stored) != 1 || stored[0].GetId() != resp.Msg.GetId() {
		t.Fatalf("persisted = %+v", stored)
	}
}

func TestSidecarTypeCreate_ErrorMatrix(t *testing.T) {
	server, _ := newTestDirectoryScannerServer(dsConfig(nil, nil))

	cases := map[string]*appconfig.SidecarTypeDefinition{
		"id set":         {Id: "chosen", Type: "x", Category: "image", Patterns: []string{"^x$"}},
		"non-zero order": {Type: "x", Category: "image", Order: 10, Patterns: []string{"^x$"}},
		"empty type":     {Category: "image", Patterns: []string{"^x$"}},
		"bad category":   {Type: "x", Category: "nonsense", Patterns: []string{"^x$"}},
		"no pattern":     {Type: "x", Category: "image"},
	}
	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := server.CreateSidecarType(context.Background(), connect.NewRequest(&metarrv1.CreateSidecarTypeRequest{SidecarType: entry}))
			if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
				t.Fatalf("code = %v, want InvalidArgument", got)
			}
		})
	}
}

func TestSidecarTypeGet_FoundAndNotFound(t *testing.T) {
	withLiveConfig(t, dsConfig(nil, []*appconfig.SidecarTypeDefinition{seededSidecarType("id-1", "poster", 10)}))
	server := &DirectoryScannerServer{Handlers: &handlers.Handlers{}}

	resp, err := server.GetSidecarType(context.Background(), connect.NewRequest(&metarrv1.GetSidecarTypeRequest{Id: "id-1"}))
	if err != nil {
		t.Fatalf("GetSidecarType: %v", err)
	}
	if resp.Msg.GetType() != "poster" {
		t.Errorf("type = %q", resp.Msg.GetType())
	}
	_, err = server.GetSidecarType(context.Background(), connect.NewRequest(&metarrv1.GetSidecarTypeRequest{Id: "nope"}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestSidecarTypeUpdate_PartialMaskAndErrors(t *testing.T) {
	seed := func() *appconfig.Config {
		return dsConfig(nil, []*appconfig.SidecarTypeDefinition{seededSidecarType("id-1", "poster", 10)})
	}

	t.Run("partial mask keeps order and siblings", func(t *testing.T) {
		server, backend := newTestDirectoryScannerServer(seed())
		_, err := server.UpdateSidecarType(context.Background(), connect.NewRequest(&metarrv1.UpdateSidecarTypeRequest{
			SidecarType: &appconfig.SidecarTypeDefinition{Id: "id-1", Category: "subtitle"},
			UpdateMask:  &fieldmaskpb.FieldMask{Paths: []string{"category"}},
		}))
		if err != nil {
			t.Fatalf("UpdateSidecarType: %v", err)
		}
		got := backend.cfg.GetDirectoryScanner().GetSidecarTypes()[0]
		if got.GetCategory() != "subtitle" {
			t.Errorf("category = %q, want subtitle", got.GetCategory())
		}
		if got.GetOrder() != 10 || got.GetType() != "poster" {
			t.Errorf("an unmasked field moved: %+v", got)
		}
	})
	t.Run("unknown id is NotFound", func(t *testing.T) {
		server, _ := newTestDirectoryScannerServer(seed())
		_, err := server.UpdateSidecarType(context.Background(), connect.NewRequest(&metarrv1.UpdateSidecarTypeRequest{
			SidecarType: &appconfig.SidecarTypeDefinition{Id: "ghost", Category: "subtitle"},
			UpdateMask:  &fieldmaskpb.FieldMask{Paths: []string{"category"}},
		}))
		if got := connect.CodeOf(err); got != connect.CodeNotFound {
			t.Fatalf("code = %v, want NotFound", got)
		}
	})
	t.Run("order in mask is InvalidArgument", func(t *testing.T) {
		server, _ := newTestDirectoryScannerServer(seed())
		_, err := server.UpdateSidecarType(context.Background(), connect.NewRequest(&metarrv1.UpdateSidecarTypeRequest{
			SidecarType: &appconfig.SidecarTypeDefinition{Id: "id-1", Order: 20},
			UpdateMask:  &fieldmaskpb.FieldMask{Paths: []string{"order"}},
		}))
		if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
			t.Fatalf("code = %v, want InvalidArgument", got)
		}
	})
	t.Run("a table that fails to compile is InvalidArgument", func(t *testing.T) {
		server, _ := newTestDirectoryScannerServer(seed())
		_, err := server.UpdateSidecarType(context.Background(), connect.NewRequest(&metarrv1.UpdateSidecarTypeRequest{
			SidecarType: &appconfig.SidecarTypeDefinition{Id: "id-1", Patterns: []string{"([broken"}},
			UpdateMask:  &fieldmaskpb.FieldMask{Paths: []string{"patterns"}},
		}))
		if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
			t.Fatalf("code = %v, want InvalidArgument", got)
		}
	})
}

func TestSidecarTypeDelete_RemovesAndNotFound(t *testing.T) {
	server, backend := newTestDirectoryScannerServer(dsConfig(nil, []*appconfig.SidecarTypeDefinition{
		seededSidecarType("id-1", "poster", 10), seededSidecarType("id-2", "fanart", 20),
	}))

	if _, err := server.DeleteSidecarType(context.Background(), connect.NewRequest(&metarrv1.DeleteSidecarTypeRequest{Id: "id-1"})); err != nil {
		t.Fatalf("DeleteSidecarType: %v", err)
	}
	if got := backend.cfg.GetDirectoryScanner().GetSidecarTypes(); len(got) != 1 || got[0].GetId() != "id-2" {
		t.Fatalf("after delete = %+v", got)
	}
	_, err := server.DeleteSidecarType(context.Background(), connect.NewRequest(&metarrv1.DeleteSidecarTypeRequest{Id: "id-1"}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestSidecarTypeReorder_ReturnsUpdatedListAndRejectsPartial(t *testing.T) {
	server, _ := newTestDirectoryScannerServer(dsConfig(nil, []*appconfig.SidecarTypeDefinition{
		seededSidecarType("id-1", "poster", 10), seededSidecarType("id-2", "fanart", 20),
	}))

	resp, err := server.ReorderSidecarTypes(context.Background(), connect.NewRequest(&metarrv1.ReorderSidecarTypesRequest{
		Orders: map[string]int32{"id-1": 20, "id-2": 10},
	}))
	if err != nil {
		t.Fatalf("ReorderSidecarTypes: %v", err)
	}
	byID := map[string]int32{}
	for _, entry := range resp.Msg.GetSidecarTypes() {
		byID[entry.GetId()] = entry.GetOrder()
	}
	if byID["id-1"] != 20 || byID["id-2"] != 10 {
		t.Errorf("returned orders = %+v, want id-1:20 id-2:10", byID)
	}

	_, err = server.ReorderSidecarTypes(context.Background(), connect.NewRequest(&metarrv1.ReorderSidecarTypesRequest{
		Orders: map[string]int32{"id-1": 10},
	}))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument for a partial order", got)
	}
}

func TestSidecarTypeReset_ReturnsDefaults(t *testing.T) {
	server, backend := newTestDirectoryScannerServer(dsConfig(nil, []*appconfig.SidecarTypeDefinition{
		seededSidecarType("id-1", "custom", 10),
	}))

	resp, err := server.ResetSidecarTypes(context.Background(), connect.NewRequest(&metarrv1.ResetSidecarTypesRequest{}))
	if err != nil {
		t.Fatalf("ResetSidecarTypes: %v", err)
	}
	want := appconfig.DefaultSidecarTypes()
	if len(resp.Msg.GetSidecarTypes()) != len(want) {
		t.Fatalf("returned %d types, want the %d defaults", len(resp.Msg.GetSidecarTypes()), len(want))
	}
	if len(backend.cfg.GetDirectoryScanner().GetSidecarTypes()) != len(want) {
		t.Errorf("persisted table was not reset to defaults")
	}
}

func TestSidecarTypeList_Paginates(t *testing.T) {
	withLiveConfig(t, dsConfig(nil, []*appconfig.SidecarTypeDefinition{
		seededSidecarType("id-1", "a", 10), seededSidecarType("id-2", "b", 20), seededSidecarType("id-3", "c", 30),
	}))
	server := &DirectoryScannerServer{Handlers: &handlers.Handlers{}}

	first, err := server.ListSidecarTypes(context.Background(), connect.NewRequest(&metarrv1.ListSidecarTypesRequest{
		PageSize: 2, OrderBy: "order",
	}))
	if err != nil {
		t.Fatalf("ListSidecarTypes: %v", err)
	}
	if len(first.Msg.GetSidecarTypes()) != 2 || first.Msg.GetNextPageToken() == "" {
		t.Fatalf("page 1 = %d types, token %q", len(first.Msg.GetSidecarTypes()), first.Msg.GetNextPageToken())
	}
	second, err := server.ListSidecarTypes(context.Background(), connect.NewRequest(&metarrv1.ListSidecarTypesRequest{
		PageSize: 2, OrderBy: "order", PageToken: first.Msg.GetNextPageToken(),
	}))
	if err != nil {
		t.Fatalf("ListSidecarTypes page 2: %v", err)
	}
	if len(second.Msg.GetSidecarTypes()) != 1 || second.Msg.GetNextPageToken() != "" {
		t.Fatalf("page 2 = %d types, token %q", len(second.Msg.GetSidecarTypes()), second.Msg.GetNextPageToken())
	}
}
