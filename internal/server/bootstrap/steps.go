package bootstrap

import (
	"crypto/rand"
	"encoding/base64"

	"Metarr/internal/shared/appconfig"
)

// directoryScannerDefaultsStep seeds the directory scanner config the first
// time the app starts against this database (a fresh install, or an
// existing database predating the directory_scanner section).
func directoryScannerDefaultsStep(cfg *appconfig.Config) (bool, error) {
	if cfg.DirectoryScanner.ParallelCount != 0 {
		return false, nil
	}
	cfg.DirectoryScanner.ParallelCount = appconfig.Default().DirectoryScanner.ParallelCount
	cfg.DirectoryScanner.ScanDirectories = []*appconfig.ScanDirectory{}
	return true, nil
}

// sidecarTypesSeedStep seeds the sidecar classification table the first
// time the app starts against this database, on the same "fresh install, or
// a database predating this section" reasoning as the scanner defaults
// step. An empty table would classify nothing, so this is what makes the
// built-in rules the starting point a user then edits.
func sidecarTypesSeedStep(cfg *appconfig.Config) (bool, error) {
	if len(cfg.DirectoryScanner.SidecarTypes) != 0 {
		return false, nil
	}
	cfg.DirectoryScanner.SidecarTypes = appconfig.DefaultSidecarTypes()
	return true, nil
}

// loggingDefaultsStep seeds the logging config for a database predating the
// logging pipeline, which would otherwise leave the level threshold at its
// zero value (every level including Debug — the opposite of the quiet
// default a fresh install gets).
func loggingDefaultsStep(cfg *appconfig.Config) (bool, error) {
	if cfg.Logging.ServerLevel != "" {
		return false, nil
	}
	cfg.Logging = appconfig.Default().Logging
	return true, nil
}

// eventBusDefaultsStep seeds the event_bus config for a database predating
// it, which would otherwise leave every knob at its zero value — a zero
// retry cap and a zero-entry MAXLEN, which would drop every message.
func eventBusDefaultsStep(cfg *appconfig.Config) (bool, error) {
	if cfg.EventBus != nil && cfg.EventBus.RetentionHours != 0 {
		return false, nil
	}
	cfg.EventBus = appconfig.Default().EventBus
	return true, nil
}

// sidecarTypesMergeMissingStep appends any built-in sidecar type added to
// the defaults after this database was first seeded, since the seed step
// only fires on an empty table. added receives how many were appended, for
// Run's report.
func sidecarTypesMergeMissingStep(added *int) func(cfg *appconfig.Config) (bool, error) {
	return func(cfg *appconfig.Config) (bool, error) {
		merged, n := appconfig.MergeMissingSidecarTypes(cfg.DirectoryScanner.SidecarTypes)
		if n == 0 {
			return false, nil
		}
		cfg.DirectoryScanner.SidecarTypes = merged
		*added = n
		return true, nil
	}
}

// hmacSecretSeedStep generates the HMAC-SHA256 signing secret the first
// time the app starts against a database with none configured. The secret is
// cryptographically random 32 bytes, base64-encoded, and stored in Auth.HmacSecret
// for use in JWT token signing and verification. generated is set to true iff
// this call actually generated a secret.
func hmacSecretSeedStep(generated *bool) func(cfg *appconfig.Config) (bool, error) {
	return func(cfg *appconfig.Config) (bool, error) {
		if cfg.Auth.HmacSecret != "" {
			return false, nil
		}

		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return false, err
		}

		cfg.Auth.HmacSecret = base64.StdEncoding.EncodeToString(secret)
		*generated = true
		return true, nil
	}
}
