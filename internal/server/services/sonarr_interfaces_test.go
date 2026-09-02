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

func newTestSonarrServer(seed *appconfig.Config) (*SonarrInterfaceServer, *fakeConfigBackend) {
	backend := &fakeConfigBackend{cfg: seed}
	store := appconfigstore.New(backend, backend, backend)
	return &SonarrInterfaceServer{Handlers: &handlers.Handlers{
		AppConfigStore: store,
		Logger:         slog.Default(),
	}}, backend
}

func seededInstance(slug string) *appconfig.SonarrInstance {
	return &appconfig.SonarrInstance{
		InstanceSlug: slug,
		InstanceName: slug + " name",
		SonarrUrl:    "http://localhost:8989",
		SonarrApiKey: "key-" + slug,
		Storage:      &appconfig.StorageConfig{Mode: "cache", Ttl: "24h"},
	}
}

func createReq(id string, inst *metarrv1.SonarrInstance) *connect.Request[metarrv1.CreateSonarrInstanceRequest] {
	return connect.NewRequest(&metarrv1.CreateSonarrInstanceRequest{
		SonarrInstanceId: id,
		SonarrInstance:   inst,
	})
}

// --- Create -------------------------------------------------------------------

func TestSonarrCreate_AppendsAndReturnsStoredResource(t *testing.T) {
	server, backend := newTestSonarrServer(&appconfig.Config{
		Admin: &appconfig.AdminUser{Username: "admin", PasswordHash: "keep-me"},
	})

	ctx := correlation.WithID(context.Background(), "corr-1")
	resp, err := server.CreateSonarrInstance(ctx, createReq("sonarr-main", &metarrv1.SonarrInstance{
		InstanceName: "Main",
		SonarrUrl:    "http://sonarr:8989",
	}))
	if err != nil {
		t.Fatalf("CreateSonarrInstance: %v", err)
	}
	if resp.Msg.GetInstanceSlug() != "sonarr-main" || resp.Msg.GetInstanceName() != "Main" {
		t.Errorf("response = %+v, want slug sonarr-main / name Main", resp.Msg)
	}
	if resp.Msg.GetStorage() == nil {
		t.Error("stored instance has a nil storage section")
	}
	stored := backend.cfg.GetInterfaces().GetSonarr()
	if len(stored) != 1 || stored[0].GetInstanceSlug() != "sonarr-main" {
		t.Fatalf("persisted instances = %+v, want one slugged sonarr-main", stored)
	}
	if backend.cfg.GetAdmin().GetPasswordHash() != "keep-me" {
		t.Errorf("a scoped Sonarr write disturbed the admin credential: %+v", backend.cfg.GetAdmin())
	}
	if len(backend.fired) != 0 {
		t.Fatalf("a synchronous write fired %d system_config_update events, want 0", len(backend.fired))
	}
}

func TestSonarrCreate_ExistingSlugIsAlreadyExists(t *testing.T) {
	server, _ := newTestSonarrServer(&appconfig.Config{
		Interfaces: &appconfig.InterfacesConfig{Sonarr: []*appconfig.SonarrInstance{seededInstance("dup")}},
	})

	_, err := server.CreateSonarrInstance(context.Background(), createReq("dup", &metarrv1.SonarrInstance{}))
	if got := connect.CodeOf(err); got != connect.CodeAlreadyExists {
		t.Fatalf("code = %v, want AlreadyExists", got)
	}
}

func TestSonarrCreate_SlugBodyMismatchIsInvalidArgument(t *testing.T) {
	server, _ := newTestSonarrServer(&appconfig.Config{})

	_, err := server.CreateSonarrInstance(context.Background(), createReq("sonarr-main", &metarrv1.SonarrInstance{
		InstanceSlug: "something-else",
	}))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", got)
	}
}

func TestSonarrCreate_MatchingBodySlugIsAccepted(t *testing.T) {
	server, _ := newTestSonarrServer(&appconfig.Config{})

	_, err := server.CreateSonarrInstance(context.Background(), createReq("sonarr-main", &metarrv1.SonarrInstance{
		InstanceSlug: "sonarr-main",
	}))
	if err != nil {
		t.Fatalf("CreateSonarrInstance with a matching body slug: %v", err)
	}
}

func TestSonarrCreate_EmptySlugIsInvalidArgument(t *testing.T) {
	server, _ := newTestSonarrServer(&appconfig.Config{})

	_, err := server.CreateSonarrInstance(context.Background(), createReq("", &metarrv1.SonarrInstance{}))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", got)
	}
}

// --- Get --------------------------------------------------------------------

func TestSonarrGet_FoundAndNotFound(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		Interfaces: &appconfig.InterfacesConfig{Sonarr: []*appconfig.SonarrInstance{seededInstance("known")}},
	})
	server := &SonarrInterfaceServer{Handlers: &handlers.Handlers{}}

	resp, err := server.GetSonarrInstance(context.Background(), connect.NewRequest(&metarrv1.GetSonarrInstanceRequest{Slug: "known"}))
	if err != nil {
		t.Fatalf("GetSonarrInstance: %v", err)
	}
	if resp.Msg.GetSonarrApiKey() != "key-known" {
		t.Errorf("api key = %q, want key-known", resp.Msg.GetSonarrApiKey())
	}
	// The response is a clone: mutating it must not reach live config.
	resp.Msg.SonarrApiKey = "tampered"
	if appconfig.Get().Interfaces.Sonarr[0].GetSonarrApiKey() != "key-known" {
		t.Error("GetSonarrInstance handed out the live-config pointer")
	}

	_, err = server.GetSonarrInstance(context.Background(), connect.NewRequest(&metarrv1.GetSonarrInstanceRequest{Slug: "nope"}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

// --- Update ---------------------------------------------------------------

func TestSonarrUpdate_DottedPathUpdatesOneNestedField(t *testing.T) {
	server, backend := newTestSonarrServer(&appconfig.Config{
		Interfaces: &appconfig.InterfacesConfig{Sonarr: []*appconfig.SonarrInstance{seededInstance("main")}},
	})

	_, err := server.UpdateSonarrInstance(context.Background(), connect.NewRequest(&metarrv1.UpdateSonarrInstanceRequest{
		SonarrInstance: &metarrv1.SonarrInstance{
			InstanceSlug: "main",
			Storage:      &metarrv1.StorageConfig{Ttl: "90m", Mode: "ignored-not-masked"},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"storage.ttl"}},
	}))
	if err != nil {
		t.Fatalf("UpdateSonarrInstance: %v", err)
	}
	got := backend.cfg.GetInterfaces().GetSonarr()[0]
	if got.GetStorage().GetTtl() != "90m" {
		t.Errorf("storage.ttl = %q, want 90m", got.GetStorage().GetTtl())
	}
	if got.GetStorage().GetMode() != "cache" {
		t.Errorf("storage.mode = %q, want the seeded cache — an unmasked nested field moved", got.GetStorage().GetMode())
	}
	if got.GetSonarrApiKey() != "key-main" {
		t.Errorf("sonarr_api_key = %q, want the seeded key-main — an unmasked field moved", got.GetSonarrApiKey())
	}
}

func TestSonarrUpdate_PartialMaskLeavesSiblingsUntouched(t *testing.T) {
	server, backend := newTestSonarrServer(&appconfig.Config{
		Interfaces: &appconfig.InterfacesConfig{Sonarr: []*appconfig.SonarrInstance{seededInstance("main")}},
	})

	_, err := server.UpdateSonarrInstance(context.Background(), connect.NewRequest(&metarrv1.UpdateSonarrInstanceRequest{
		SonarrInstance: &metarrv1.SonarrInstance{
			InstanceSlug: "main",
			InstanceName: "Renamed",
			SonarrUrl:    "http://wrong:1",
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"instance_name"}},
	}))
	if err != nil {
		t.Fatalf("UpdateSonarrInstance: %v", err)
	}
	got := backend.cfg.GetInterfaces().GetSonarr()[0]
	if got.GetInstanceName() != "Renamed" {
		t.Errorf("instance_name = %q, want Renamed", got.GetInstanceName())
	}
	if got.GetSonarrUrl() != "http://localhost:8989" {
		t.Errorf("sonarr_url = %q, want the seeded value — an unmasked field moved", got.GetSonarrUrl())
	}
}

func TestSonarrUpdate_RejectsEmptyMask(t *testing.T) {
	server, _ := newTestSonarrServer(&appconfig.Config{
		Interfaces: &appconfig.InterfacesConfig{Sonarr: []*appconfig.SonarrInstance{seededInstance("main")}},
	})

	_, err := server.UpdateSonarrInstance(context.Background(), connect.NewRequest(&metarrv1.UpdateSonarrInstanceRequest{
		SonarrInstance: &metarrv1.SonarrInstance{InstanceSlug: "main", InstanceName: "x"},
	}))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", got)
	}
}

func TestSonarrUpdate_RejectsUnknownPath(t *testing.T) {
	server, _ := newTestSonarrServer(&appconfig.Config{
		Interfaces: &appconfig.InterfacesConfig{Sonarr: []*appconfig.SonarrInstance{seededInstance("main")}},
	})

	for name, path := range map[string]string{
		"no such field":          "instance_nickname",
		"descend through scalar": "instance_name.value",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := server.UpdateSonarrInstance(context.Background(), connect.NewRequest(&metarrv1.UpdateSonarrInstanceRequest{
				SonarrInstance: &metarrv1.SonarrInstance{InstanceSlug: "main"},
				UpdateMask:     &fieldmaskpb.FieldMask{Paths: []string{path}},
			}))
			if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
				t.Fatalf("code = %v, want InvalidArgument", got)
			}
		})
	}
}

func TestSonarrUpdate_UnknownSlugIsNotFoundWithoutAllowMissing(t *testing.T) {
	server, _ := newTestSonarrServer(&appconfig.Config{})

	_, err := server.UpdateSonarrInstance(context.Background(), connect.NewRequest(&metarrv1.UpdateSonarrInstanceRequest{
		SonarrInstance: &metarrv1.SonarrInstance{InstanceSlug: "ghost", InstanceName: "x"},
		UpdateMask:     &fieldmaskpb.FieldMask{Paths: []string{"instance_name"}},
	}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestSonarrUpdate_AllowMissingCreatesOnUnknownSlug(t *testing.T) {
	server, backend := newTestSonarrServer(&appconfig.Config{})

	resp, err := server.UpdateSonarrInstance(context.Background(), connect.NewRequest(&metarrv1.UpdateSonarrInstanceRequest{
		SonarrInstance: &metarrv1.SonarrInstance{InstanceSlug: "fresh", InstanceName: "Fresh"},
		// An unknown path in the mask is ignored on the allow_missing create branch.
		UpdateMask:   &fieldmaskpb.FieldMask{Paths: []string{"bogus"}},
		AllowMissing: true,
	}))
	if err != nil {
		t.Fatalf("UpdateSonarrInstance allow_missing: %v", err)
	}
	if resp.Msg.GetInstanceSlug() != "fresh" || resp.Msg.GetInstanceName() != "Fresh" {
		t.Errorf("response = %+v, want the whole resource created", resp.Msg)
	}
	if got := backend.cfg.GetInterfaces().GetSonarr(); len(got) != 1 || got[0].GetInstanceSlug() != "fresh" {
		t.Fatalf("persisted = %+v, want one instance slugged fresh", got)
	}
}

// --- Delete -------------------------------------------------------------------

func TestSonarrDelete_RemovesAndIsNotFoundForUnknown(t *testing.T) {
	server, backend := newTestSonarrServer(&appconfig.Config{
		Interfaces: &appconfig.InterfacesConfig{Sonarr: []*appconfig.SonarrInstance{
			seededInstance("a"), seededInstance("b"),
		}},
	})

	if _, err := server.DeleteSonarrInstance(context.Background(), connect.NewRequest(&metarrv1.DeleteSonarrInstanceRequest{Slug: "a"})); err != nil {
		t.Fatalf("DeleteSonarrInstance: %v", err)
	}
	got := backend.cfg.GetInterfaces().GetSonarr()
	if len(got) != 1 || got[0].GetInstanceSlug() != "b" {
		t.Fatalf("after delete = %+v, want only b", got)
	}

	_, err := server.DeleteSonarrInstance(context.Background(), connect.NewRequest(&metarrv1.DeleteSonarrInstanceRequest{Slug: "a"}))
	if code := connect.CodeOf(err); code != connect.CodeNotFound {
		t.Fatalf("code = %v, want NotFound", code)
	}
}

// --- List -------------------------------------------------------------------

func TestSonarrList_PaginatesAndRoundTripsTheToken(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		Interfaces: &appconfig.InterfacesConfig{Sonarr: []*appconfig.SonarrInstance{
			seededInstance("a"), seededInstance("b"), seededInstance("c"),
		}},
	})
	server := &SonarrInterfaceServer{Handlers: &handlers.Handlers{}}

	first, err := server.ListSonarrInstances(context.Background(), connect.NewRequest(&metarrv1.ListSonarrInstancesRequest{
		PageSize: 2, OrderBy: "instance_slug",
	}))
	if err != nil {
		t.Fatalf("ListSonarrInstances page 1: %v", err)
	}
	if len(first.Msg.GetSonarrInstances()) != 2 || first.Msg.GetNextPageToken() == "" {
		t.Fatalf("page 1 = %d instances, token %q; want 2 and a non-empty token",
			len(first.Msg.GetSonarrInstances()), first.Msg.GetNextPageToken())
	}

	second, err := server.ListSonarrInstances(context.Background(), connect.NewRequest(&metarrv1.ListSonarrInstancesRequest{
		PageSize: 2, OrderBy: "instance_slug", PageToken: first.Msg.GetNextPageToken(),
	}))
	if err != nil {
		t.Fatalf("ListSonarrInstances page 2: %v", err)
	}
	if len(second.Msg.GetSonarrInstances()) != 1 || second.Msg.GetNextPageToken() != "" {
		t.Fatalf("page 2 = %d instances, token %q; want 1 and an empty token",
			len(second.Msg.GetSonarrInstances()), second.Msg.GetNextPageToken())
	}
	if second.Msg.GetSonarrInstances()[0].GetInstanceSlug() != "c" {
		t.Errorf("page 2 instance = %q, want c", second.Msg.GetSonarrInstances()[0].GetInstanceSlug())
	}
}

func TestSonarrList_OrderByDescendingAndUnknownField(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		Interfaces: &appconfig.InterfacesConfig{Sonarr: []*appconfig.SonarrInstance{
			seededInstance("a"), seededInstance("c"), seededInstance("b"),
		}},
	})
	server := &SonarrInterfaceServer{Handlers: &handlers.Handlers{}}

	resp, err := server.ListSonarrInstances(context.Background(), connect.NewRequest(&metarrv1.ListSonarrInstancesRequest{
		OrderBy: "instance_slug desc",
	}))
	if err != nil {
		t.Fatalf("ListSonarrInstances: %v", err)
	}
	got := []string{}
	for _, i := range resp.Msg.GetSonarrInstances() {
		got = append(got, i.GetInstanceSlug())
	}
	if want := []string{"c", "b", "a"}; !equalStrings(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}

	_, err = server.ListSonarrInstances(context.Background(), connect.NewRequest(&metarrv1.ListSonarrInstancesRequest{
		OrderBy: "no_such_field",
	}))
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", code)
	}
}

func TestSonarrList_UnsupportedFilterIsUnimplemented(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{})
	server := &SonarrInterfaceServer{Handlers: &handlers.Handlers{}}

	_, err := server.ListSonarrInstances(context.Background(), connect.NewRequest(&metarrv1.ListSonarrInstancesRequest{
		Filter: `instance_name = "x"`,
	}))
	if code := connect.CodeOf(err); code != connect.CodeUnimplemented {
		t.Fatalf("code = %v, want Unimplemented", code)
	}
}

// --- Cross-interface-type slug uniqueness -----------------------------------

// checkSlugFreeAcrossInterfaces is the write-path guard for the rule that a
// slug is unique across every interface type, not just within Sonarr. Today
// Sonarr is the only type, so the RPC path reports a clash as AlreadyExists
// before this runs; the helper still maps a genuine cross-type collision to
// FailedPrecondition (docs/adr/0010).
func TestSonarrCheckSlugFreeAcrossInterfaces_FailedPrecondition(t *testing.T) {
	cfg := &appconfig.Config{
		Interfaces: &appconfig.InterfacesConfig{Sonarr: []*appconfig.SonarrInstance{seededInstance("taken")}},
	}
	if err := checkSlugFreeAcrossInterfaces(cfg, "taken"); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition", connect.CodeOf(err))
	}
	if err := checkSlugFreeAcrossInterfaces(cfg, "free"); err != nil {
		t.Fatalf("unexpected error for a free slug: %v", err)
	}
}

// --- Synchronous propagation ----------------------------------------------

// The synchronous write propagates in-process before it returns, so the very
// next GetSonarrInstance already reflects it with no system_config_update
// round trip.
func TestSonarrCreate_VisibleOnNextGet(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{})
	server, _ := newTestSonarrServer(&appconfig.Config{})
	server.AppConfigStore.SetPropagator(liveConfigPropagator{})

	if _, err := server.CreateSonarrInstance(context.Background(), createReq("live", &metarrv1.SonarrInstance{InstanceName: "Live"})); err != nil {
		t.Fatalf("CreateSonarrInstance: %v", err)
	}

	got, err := server.GetSonarrInstance(context.Background(), connect.NewRequest(&metarrv1.GetSonarrInstanceRequest{Slug: "live"}))
	if err != nil {
		t.Fatalf("GetSonarrInstance after Create: %v", err)
	}
	if got.Msg.GetInstanceName() != "Live" {
		t.Errorf("Get after Create returned name %q, want Live", got.Msg.GetInstanceName())
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
