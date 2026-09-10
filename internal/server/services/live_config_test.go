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
//
// cfg is Normalized first, matching production: every path that sets live
// config (bootstrap warm-up, the config propagator) Normalizes beforehand,
// so a read site can use plain field access.
func withLiveConfig(t *testing.T, cfg *appconfig.Config) {
	t.Helper()
	previous := appconfig.Get()
	appconfig.Set(appconfig.Normalize(cfg))
	t.Cleanup(func() { appconfig.Set(previous) })
}

// These three exercise a representative sample of the twelve handlers
// converted in issue #14 — a plain read-and-convert (ConfigServer.Get), a
// lookup-by-slug with branching (DirectoryScannerServer.GetScanDirectory), and a
// list conversion in a different file (SonarrInterfaceServer.ListSonarrInstances) — to
// prove the live-config seam works, not to re-test every one of the twelve
// individually.

func TestConfigServerGet_ReadsLiveConfig(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		Admin: &appconfig.AdminUser{Username: "arranged-admin", Email: "arranged@example.com"},
	})

	server := &ConfigServer{Handlers: &handlers.Handlers{}}

	resp, err := server.GetConfig(context.Background(), connect.NewRequest(&metarrv1.GetConfigRequest{}))
	if err != nil {
		t.Fatalf("GetConfig returned error: %v", err)
	}
	if got := resp.Msg.GetConfig().GetAdmin().GetUsername(); got != "arranged-admin" {
		t.Errorf("admin username = %q, want %q", got, "arranged-admin")
	}
}

// TestConfigServerGet_RedactsSecretsWithoutDisturbingLiveConfig is the one
// genuinely new invariant of the config generation slice: a read of the
// application config returns blanked secrets — admin credentials and the JWT
// signing secret alike — AND leaves the stored values intact. Both halves
// matter: live config holds the running server's own password hash and HMAC
// secret, and a redaction that mutated either in place would lock the
// administrator out and invalidate every issued token until the next reload.
// The HMAC-secret half also closes an active leak — the value that can forge
// an admin JWT for any username previously shipped to the browser on every
// config-page load.
func TestConfigServerGet_RedactsSecretsWithoutDisturbingLiveConfig(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		Admin: &appconfig.AdminUser{
			Username:     "admin",
			Email:        "admin@example.com",
			PasswordSalt: "live-salt",
			PasswordHash: "live-hash",
		},
		Auth: &appconfig.AuthConfig{HmacSecret: "live-hmac-secret"},
	})

	server := &ConfigServer{Handlers: &handlers.Handlers{}}

	resp, err := server.GetConfig(context.Background(), connect.NewRequest(&metarrv1.GetConfigRequest{}))
	if err != nil {
		t.Fatalf("GetConfig returned error: %v", err)
	}

	admin := resp.Msg.GetConfig().GetAdmin()
	if admin.GetPasswordSalt() != "" || admin.GetPasswordHash() != "" {
		t.Errorf("response carried admin credentials: salt=%q hash=%q", admin.GetPasswordSalt(), admin.GetPasswordHash())
	}
	if admin.GetUsername() != "admin" || admin.GetEmail() != "admin@example.com" {
		t.Errorf("response dropped the admin identity: %+v", admin)
	}
	if got := resp.Msg.GetConfig().GetAuth().GetHmacSecret(); got != "" {
		t.Errorf("response carried the HMAC signing secret: %q", got)
	}

	live := appconfig.Get()
	if live.Admin.PasswordSalt != "live-salt" || live.Admin.PasswordHash != "live-hash" {
		t.Fatalf("a config read erased the running server's own credentials: %+v", live.Admin)
	}
	if live.Auth.HmacSecret != "live-hmac-secret" {
		t.Fatalf("a config read erased the running server's own HMAC secret: %q", live.Auth.HmacSecret)
	}
}

func TestDirectoryScannerServerGetDirectory_ReadsLiveConfig(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		DirectoryScanner: &appconfig.DirectoryScannerConfig{
			ScanDirectories: []*appconfig.ScanDirectory{
				{ScannerSlug: "arranged-slug", ScanType: "movie", Directory: "/arranged/movies"},
			},
		},
	})

	server := &DirectoryScannerServer{Handlers: &handlers.Handlers{}}

	resp, err := server.GetScanDirectory(context.Background(), connect.NewRequest(&metarrv1.GetScanDirectoryRequest{
		Slug: "arranged-slug",
	}))
	if err != nil {
		t.Fatalf("GetScanDirectory returned error: %v", err)
	}
	if got := resp.Msg.GetDirectory(); got != "/arranged/movies" {
		t.Errorf("directory = %q, want %q", got, "/arranged/movies")
	}
}

func TestSonarrInterfaceServerList_ReadsLiveConfig(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{
		Interfaces: &appconfig.InterfacesConfig{
			Sonarr: []*appconfig.SonarrInstance{
				{InstanceSlug: "arranged-sonarr", InstanceName: "Arranged Sonarr"},
			},
		},
	})

	server := &SonarrInterfaceServer{Handlers: &handlers.Handlers{}}

	resp, err := server.ListSonarrInstances(context.Background(), connect.NewRequest(&metarrv1.ListSonarrInstancesRequest{}))
	if err != nil {
		t.Fatalf("ListSonarrInstances returned error: %v", err)
	}
	instances := resp.Msg.GetSonarrInstances()
	if len(instances) != 1 || instances[0].GetInstanceSlug() != "arranged-sonarr" {
		t.Errorf("instances = %v, want one instance slugged %q", instances, "arranged-sonarr")
	}
}
