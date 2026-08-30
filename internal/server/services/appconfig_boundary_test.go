package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoDirectStorageConfigReads guards against services reaching for the
// application config's direct-storage reader again. Every read here goes
// through live config (appconfig.Get()) instead of the config store's own
// storage-reading capability, which exists only to serve startup bootstrap
// — see CONTEXT.md's "Live config" and "Config store" entries.
//
// This is a text-level check, not a build-graph one like
// internal/agent/boundary_test.go: services legitimately imports
// internal/server/mongostore for other repositories (directories, Sonarr
// instances), so a package-level dependency ban would false-positive on
// those, not just on the config reader this guards against.
func TestNoDirectStorageConfigReads(t *testing.T) {
	const forbidden = "AppConfigRepo"
	const thisFile = "appconfig_boundary_test.go"

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing package files: %v", err)
	}

	for _, path := range files {
		if filepath.Base(path) == thisFile {
			continue
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if strings.Contains(string(contents), forbidden) {
			t.Errorf("%s references %s; services reads must use appconfig.Get() (live config), not the direct-storage reader", path, forbidden)
		}
	}
}
