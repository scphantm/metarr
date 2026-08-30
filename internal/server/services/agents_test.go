package services

import (
	"net/http"
	"testing"

	"Metarr/internal/shared/appconfig"
)

func configWithTwoLibraries() *appconfig.Config {
	return &appconfig.Config{
		DirectoryScanner: &appconfig.DirectoryScannerConfig{
			ScanDirectories: []*appconfig.ScanDirectory{
				{ScannerSlug: "movies", ScanType: "movie", Directory: "/media/movies"},
				{ScannerSlug: "tv", ScanType: "tv", Directory: "/media/tv"},
			},
		},
		Agents: []*appconfig.AgentConfig{{
			Slug: "nas-01",
			Mappings: []*appconfig.AgentDirectoryMapping{
				{ScannerSlug: "movies", AgentPath: "/mnt/tank/movies"},
			},
		}},
	}
}

func TestValidateMappingsAcceptsAnUnclaimedLibrary(t *testing.T) {
	entry := &appconfig.AgentConfig{
		Slug: "desktop",
		Mappings: []*appconfig.AgentDirectoryMapping{
			{ScannerSlug: "tv", AgentPath: "/srv/tv"},
		},
	}

	if status, err := validateMappings(configWithTwoLibraries(), entry); err != nil {
		t.Errorf("validateMappings = %d, %v; want accepted", status, err)
	}
}

// Re-saving an agent must not trip the ownership check on its own mappings,
// which is what every edit through the UI does.
func TestValidateMappingsLetsAnAgentKeepItsOwnLibrary(t *testing.T) {
	entry := &appconfig.AgentConfig{
		Slug: "nas-01",
		Mappings: []*appconfig.AgentDirectoryMapping{
			{ScannerSlug: "movies", AgentPath: "/mnt/tank/movies-renamed"},
		},
	}

	if status, err := validateMappings(configWithTwoLibraries(), entry); err != nil {
		t.Errorf("validateMappings = %d, %v; want accepted", status, err)
	}
}

// Two agents scanning one library would each overwrite the other's records
// with its own view of the same files, so the second mapping is a conflict.
func TestValidateMappingsRejectsALibraryClaimedByAnotherAgent(t *testing.T) {
	entry := &appconfig.AgentConfig{
		Slug: "desktop",
		Mappings: []*appconfig.AgentDirectoryMapping{
			{ScannerSlug: "movies", AgentPath: "/srv/movies"},
		},
	}

	status, err := validateMappings(configWithTwoLibraries(), entry)
	if err == nil {
		t.Fatal("validateMappings accepted a library already mapped to another agent")
	}
	if status != http.StatusConflict {
		t.Errorf("status = %d, want %d", status, http.StatusConflict)
	}
}

func TestValidateMappingsRejectsAnUnknownScanDirectory(t *testing.T) {
	entry := &appconfig.AgentConfig{
		Slug: "desktop",
		Mappings: []*appconfig.AgentDirectoryMapping{
			{ScannerSlug: "does-not-exist", AgentPath: "/srv/whatever"},
		},
	}

	status, err := validateMappings(configWithTwoLibraries(), entry)
	if err == nil {
		t.Fatal("validateMappings accepted a mapping to a scan directory that does not exist")
	}
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

// A duplicate would make the projection ambiguous about which path to walk.
func TestValidateMappingsRejectsTheSameLibraryTwice(t *testing.T) {
	entry := &appconfig.AgentConfig{
		Slug: "desktop",
		Mappings: []*appconfig.AgentDirectoryMapping{
			{ScannerSlug: "tv", AgentPath: "/srv/tv"},
			{ScannerSlug: "tv", AgentPath: "/srv/tv-other"},
		},
	}

	status, err := validateMappings(configWithTwoLibraries(), entry)
	if err == nil {
		t.Fatal("validateMappings accepted the same scan directory mapped twice")
	}
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

// An agent with no mappings is a valid, useful state: it is how you register a
// machine before deciding what it may see.
func TestValidateMappingsAcceptsAnAgentWithNoMappings(t *testing.T) {
	entry := &appconfig.AgentConfig{Slug: "brand-new"}

	if status, err := validateMappings(configWithTwoLibraries(), entry); err != nil {
		t.Errorf("validateMappings = %d, %v; want accepted", status, err)
	}
}

func TestAgentForScannerFindsTheOwningAgent(t *testing.T) {
	config := configWithTwoLibraries()

	agent, ok := appconfig.AgentForScanner(config, "movies")
	if !ok || agent.Slug != "nas-01" {
		t.Errorf("AgentForScanner(movies) = %+v, %v; want nas-01", agent, ok)
	}
	if _, ok := appconfig.AgentForScanner(config, "tv"); ok {
		t.Error("AgentForScanner claimed an owner for an unmapped library")
	}
}
