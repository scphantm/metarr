package appconfig

import "testing"

// Normalize is what lets every read site in the server use plain field access
// on a config section. These tests cover the cases that reach it from a
// decoded document rather than from Default().

func TestNormalizeFillsEveryNilSection(t *testing.T) {
	config := Normalize(&Config{})

	if config.ApiKeys == nil {
		t.Error("ApiKeys is nil")
	}
	if config.Admin == nil {
		t.Error("Admin is nil")
	}
	if config.Interfaces == nil {
		t.Error("Interfaces is nil")
	}
	if config.DirectoryScanner == nil {
		t.Error("DirectoryScanner is nil")
	}
	if config.Logging == nil {
		t.Error("Logging is nil")
	}
}

// A nil config is the one case Normalize answers with defaults rather than
// empty sections, so callers must use the returned value rather than relying
// on the argument having been mutated.
func TestNormalizeReturnsDefaultsForANilConfig(t *testing.T) {
	config := Normalize(nil)

	if config == nil {
		t.Fatal("Normalize(nil) returned nil")
	}
	if config.DirectoryScanner.ParallelCount == 0 {
		t.Error("Normalize(nil) did not return the defaults")
	}
}

// Normalize fills in what is absent; it must never overwrite what a document
// actually said. Confusing the two would turn it into a second, silent
// source of defaults competing with bootstrap.
func TestNormalizeLeavesPopulatedSectionsAlone(t *testing.T) {
	config := Normalize(&Config{
		Logging:          &LoggingConfig{ServerLevel: LogLevelDebug},
		DirectoryScanner: &DirectoryScannerConfig{ParallelCount: 3},
	})

	if config.Logging.ServerLevel != LogLevelDebug {
		t.Errorf("ServerLevel = %q, want %q", config.Logging.ServerLevel, LogLevelDebug)
	}
	if config.DirectoryScanner.ParallelCount != 3 {
		t.Errorf("ParallelCount = %d, want 3", config.DirectoryScanner.ParallelCount)
	}
	// An empty table is a real answer — the operator deleted every entry —
	// and must not be refilled with the built-in types.
	if len(config.DirectoryScanner.SidecarTypes) != 0 {
		t.Errorf("SidecarTypes = %d entries, want 0; Normalize must not apply defaults",
			len(config.DirectoryScanner.SidecarTypes))
	}
}

// A document written before authentication_scheme existed, or a freshly
// seeded one, carries the zero value. The config layer — not just
// builtin_defaults.json — guarantees the default resolves to None so the
// auth interceptor can compare against a concrete scheme (docs/adr/0012).
func TestNormalizeDefaultsAnUnsetAuthenticationSchemeToNone(t *testing.T) {
	config := Normalize(&Config{})

	if got := config.Admin.GetAuthenticationScheme(); got != AuthSchemeNone {
		t.Errorf("authentication_scheme = %v, want %v", got, AuthSchemeNone)
	}
}

// Normalize fills what is absent; a document that actually selected Password
// must be left exactly as it is.
func TestNormalizeLeavesAChosenAuthenticationSchemeAlone(t *testing.T) {
	config := Normalize(&Config{Admin: &AdminUser{AuthenticationScheme: AuthSchemePassword}})

	if got := config.Admin.GetAuthenticationScheme(); got != AuthSchemePassword {
		t.Errorf("authentication_scheme = %v, want %v", got, AuthSchemePassword)
	}
}

func TestNormalizeFillsAMissingStorageSection(t *testing.T) {
	config := Normalize(&Config{
		Interfaces: &InterfacesConfig{Sonarr: []*SonarrInstance{{InstanceSlug: "main"}}},
	})

	if config.Interfaces.Sonarr[0].Storage == nil {
		t.Error("Storage is nil; reading an instance's retention mode would fail")
	}
}

// A null entry in the stored array decodes to a nil instance. That is a
// malformed document rather than a shape to repair, and Normalize runs on
// every config read — so it has to skip the entry rather than bring the
// server down on startup.
func TestNormalizeSkipsANilSonarrInstance(t *testing.T) {
	config := Normalize(&Config{
		Interfaces: &InterfacesConfig{Sonarr: []*SonarrInstance{nil, {InstanceSlug: "main"}}},
	})

	if config.Interfaces.Sonarr[1].Storage == nil {
		t.Error("the instance after the nil entry was not normalized")
	}
}
