package agentregistry

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"Metarr/internal/shared/appconfig"
)

// secrets are planted throughout the config so the redaction test can look for
// them by value. Each is distinctive enough that finding one in the encoded
// projection is unambiguous.
const (
	adminKeySecret    = "SECRET-admin-api-key"
	userKeySecret     = "SECRET-user-api-key"
	webhookKeySecret  = "SECRET-webhook-api-key"
	readOnlyKeySecret = "SECRET-readonly-api-key"
	passwordHash      = "SECRET-password-hash"
	passwordSalt      = "SECRET-password-salt"
	sonarrKeySecret   = "SECRET-sonarr-api-key"
	sonarrURLSecret   = "SECRET-sonarr-url.example.com"
)

func configWithEverySecret() *appconfig.Config {
	return &appconfig.Config{
		ID: appconfig.SingletonID,
		APIKeys: appconfig.APIKeysConfig{
			Admin:    []appconfig.APIKeyEntry{{Name: "admin", Key: adminKeySecret}},
			User:     []appconfig.APIKeyEntry{{Name: "user", Key: userKeySecret}},
			Webhook:  []appconfig.APIKeyEntry{{Name: "webhook", Key: webhookKeySecret}},
			ReadOnly: []appconfig.APIKeyEntry{{Name: "readonly", Key: readOnlyKeySecret}},
		},
		Admin: appconfig.AdminUser{
			Username:     "admin",
			Email:        "admin@example.com",
			PasswordHash: passwordHash,
			PasswordSalt: passwordSalt,
		},
		Interfaces: appconfig.InterfacesConfig{
			Sonarr: []appconfig.SonarrInstance{{
				InstanceSlug: "main",
				SonarrURL:    "https://" + sonarrURLSecret,
				SonarrAPIKey: sonarrKeySecret,
			}},
		},
		DirectoryScanner: appconfig.DirectoryScannerConfig{
			ParallelCount: 8,
			ScanDirectories: []appconfig.ScanDirectory{
				{ScannerSlug: "movies", ScanType: "movie", Directory: "/media/movies"},
				{ScannerSlug: "tv", ScanType: "tv", Directory: "/media/tv"},
			},
			SidecarTypes: appconfig.DefaultSidecarTypes(),
		},
		Agents: []appconfig.AgentConfig{{
			Slug:        "nas-01",
			DisplayName: "The NAS",
			Mappings: []appconfig.AgentDirectoryMapping{
				{ScannerSlug: "movies", AgentPath: "/mnt/tank/movies"},
			},
		}},
	}
}

// TestProjectionNeverCarriesASecret is the guard on the whole agent design.
//
// An agent runs on a machine the server does not control, and the config it is
// derived from holds every credential the system has. If this test is ever
// deleted or weakened, the next field added to Config is a credential shipped
// to every agent host. It checks the encoded bytes rather than the struct, so
// a secret reaching an agent through a nested type or a future field is caught
// just as well as a direct copy.
func TestProjectionNeverCarriesASecret(t *testing.T) {
	config := configWithEverySecret()

	for _, slug := range []string{"nas-01", "unconfigured-agent"} {
		projection := BuildProjection(config, slug, time.Now())

		encoded, err := json.Marshal(projection)
		if err != nil {
			t.Fatalf("marshalling projection: %v", err)
		}
		body := string(encoded)

		for _, secret := range []string{
			adminKeySecret, userKeySecret, webhookKeySecret, readOnlyKeySecret,
			passwordHash, passwordSalt, sonarrKeySecret, sonarrURLSecret,
		} {
			if strings.Contains(body, secret) {
				t.Errorf("projection for %q leaked %q:\n%s", slug, secret, body)
			}
		}
	}
}

// An agent is told about the libraries it was mapped to and no others. Knowing
// that /media/tv exists is not dangerous, but it is not the agent's business
// and would show a remote operator the shape of storage they cannot reach.
func TestProjectionCarriesOnlyMappedLibraries(t *testing.T) {
	projection := BuildProjection(configWithEverySecret(), "nas-01", time.Now())

	if len(projection.Directories) != 1 {
		t.Fatalf("got %d directories, want 1: %+v", len(projection.Directories), projection.Directories)
	}

	directory := projection.Directories[0]
	if directory.ScannerSlug != "movies" {
		t.Errorf("scanner slug = %q, want movies", directory.ScannerSlug)
	}
	// The agent gets its own path, never the server's.
	if directory.AgentPath != "/mnt/tank/movies" {
		t.Errorf("agent path = %q, want /mnt/tank/movies", directory.AgentPath)
	}
	if encoded, _ := json.Marshal(projection); strings.Contains(string(encoded), "/media/movies") {
		t.Error("projection carried the server's own path for the library")
	}
}

// A connected agent nobody has configured yet must get a usable projection
// with no libraries, not an error. This is the state every agent starts in.
func TestProjectionForUnconfiguredAgentIsEmptyButValid(t *testing.T) {
	projection := BuildProjection(configWithEverySecret(), "brand-new", time.Now())

	if projection.Slug != "brand-new" {
		t.Errorf("slug = %q, want brand-new", projection.Slug)
	}
	if len(projection.Directories) != 0 {
		t.Errorf("got %d directories, want none", len(projection.Directories))
	}
	// The sidecar table still travels: it is not agent-specific, and an agent
	// configured a moment later should not have to wait for a second push.
	if len(projection.SidecarTypes) == 0 {
		t.Error("projection carried no sidecar table")
	}
}

// A mapping whose scan directory has been deleted, or which was left blank to
// mean "this agent cannot see this library", must not reach the agent as a
// half-formed entry it would then fail to walk.
func TestProjectionSkipsUnusableMappings(t *testing.T) {
	config := configWithEverySecret()
	config.Agents[0].Mappings = []appconfig.AgentDirectoryMapping{
		{ScannerSlug: "movies", AgentPath: "/mnt/tank/movies"},
		{ScannerSlug: "deleted-scanner", AgentPath: "/mnt/tank/gone"},
		{ScannerSlug: "tv", AgentPath: ""},
	}

	projection := BuildProjection(config, "nas-01", time.Now())

	if len(projection.Directories) != 1 {
		t.Fatalf("got %d directories, want only the usable one: %+v",
			len(projection.Directories), projection.Directories)
	}
	if projection.Directories[0].ScannerSlug != "movies" {
		t.Errorf("kept the wrong mapping: %+v", projection.Directories[0])
	}
}

// The scan type has to travel with the mapping: it lives on the scan directory
// on the server, and without it the agent cannot parse the library at all.
func TestProjectionCarriesScanTypeFromTheScanDirectory(t *testing.T) {
	projection := BuildProjection(configWithEverySecret(), "nas-01", time.Now())

	if got := projection.Directories[0].ScanType; got != "movie" {
		t.Errorf("scan type = %q, want movie", got)
	}
}
