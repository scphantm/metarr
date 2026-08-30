package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/handlers"
	"Metarr/internal/shared/appconfig"
)

// withLiveConfig arranges cfg as the live config for the duration of the
// test, restoring whatever was there before once the test ends — the
// arrange/cleanup technique described in issue #14, since appconfig.Get()
// is a process-wide singleton no existing test in this package touches.
func withLiveConfig(t *testing.T, cfg *appconfig.Config) {
	t.Helper()
	previous := appconfig.Get()
	appconfig.Set(cfg)
	t.Cleanup(func() { appconfig.Set(previous) })
}

// These three exercise a representative sample of the twelve handlers
// converted in issue #14 — a plain read-and-convert (ConfigServer.Get), a
// lookup-by-slug with branching (DirectoryScannerServer.GetDirectory), and a
// list conversion in a different file (SonarrInterfaceServer.List) — to
// prove the live-config seam works, not to re-test every one of the twelve
// individually.

func TestConfigServerGet_ReadsLiveConfig(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		Admin: appconfig.AdminUser{Username: "arranged-admin", Email: "arranged@example.com"},
	})

	server := &ConfigServer{Handlers: &handlers.Handlers{}}

	resp, err := server.Get(context.Background(), connect.NewRequest(&metarrv1.ConfigServiceGetRequest{}))
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got := resp.Msg.GetConfig().GetAdmin().GetUsername(); got != "arranged-admin" {
		t.Errorf("admin username = %q, want %q", got, "arranged-admin")
	}
}

func TestDirectoryScannerServerGetDirectory_ReadsLiveConfig(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		DirectoryScanner: appconfig.DirectoryScannerConfig{
			ScanDirectories: []appconfig.ScanDirectory{
				{ScannerSlug: "arranged-slug", ScanType: "movie", Directory: "/arranged/movies"},
			},
		},
	})

	server := &DirectoryScannerServer{Handlers: &handlers.Handlers{}}

	resp, err := server.GetDirectory(context.Background(), connect.NewRequest(&metarrv1.DirectoryScannerServiceGetDirectoryRequest{
		Slug: "arranged-slug",
	}))
	if err != nil {
		t.Fatalf("GetDirectory returned error: %v", err)
	}
	if got := resp.Msg.GetDirectory().GetDirectory(); got != "/arranged/movies" {
		t.Errorf("directory = %q, want %q", got, "/arranged/movies")
	}
}

func TestSonarrInterfaceServerList_ReadsLiveConfig(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		Interfaces: appconfig.InterfacesConfig{
			Sonarr: []appconfig.SonarrInstance{
				{InstanceSlug: "arranged-sonarr", InstanceName: "Arranged Sonarr"},
			},
		},
	})

	server := &SonarrInterfaceServer{Handlers: &handlers.Handlers{}}

	resp, err := server.List(context.Background(), connect.NewRequest(&metarrv1.SonarrInterfaceServiceListRequest{}))
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	instances := resp.Msg.GetInstances()
	if len(instances) != 1 || instances[0].GetInstanceSlug() != "arranged-sonarr" {
		t.Errorf("instances = %v, want one instance slugged %q", instances, "arranged-sonarr")
	}
}
