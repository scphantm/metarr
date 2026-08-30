// Package bootstrap seeds the application config at server startup: API
// keys, the admin account, directory-scanner defaults, the sidecar
// classification table, and the handful of one-time backfills earlier
// releases needed. See docs/adr/0003-bootstrap-is-synchronous-not-a-mutation.md
// for why this runs synchronously, straight to storage, before any listener
// exists to persist a fired event, and
// docs/adr/0004-bootstrap-module-and-embedded-defaults-file.md for why
// seeding lives in its own package, backed by one embedded defaults file,
// instead of growing appconfigstore.Store.
package bootstrap

import (
	"context"
	"fmt"

	"Metarr/internal/server/appconfigstore"
	"Metarr/internal/shared/appconfig"
)

// Report summarizes what Run actually seeded this startup. A caller uses it
// to decide what one-time output to produce (freshly generated secrets are
// shown exactly once) — Run itself never prints or writes files, so it stays
// testable without capturing stdout. A zero field means that step found
// nothing to seed this run, which is the ordinary case on every restart
// after the first.
type Report struct {
	// APIKeys is non-nil only when this run generated the four default API
	// key categories for the first time. The plaintext keys live only here
	// and in the persisted document — Metarr never stores them anywhere
	// they could be re-displayed later.
	APIKeys *appconfig.APIKeysConfig

	// Admin reports whether an admin account was created or recovered, and
	// carries the plaintext password when either happened.
	Admin appconfigstore.AdminSeedResult

	// SidecarTypesAdded is how many built-in sidecar types this run added
	// to a table that predates them (0 on an ordinary restart).
	SidecarTypesAdded int

	// APIKeyIDsBackfilled is how many stored API key entries this run
	// minted a missing id for (0 once every entry has one).
	APIKeyIDsBackfilled int
}

// Run seeds every part of the application config a fresh or upgrading
// install needs before the server starts serving requests. Each step
// persists through store.Bootstrap, the same synchronous, single-writer
// primitive appconfigstore.Store already provides — this package adds no
// new exported methods to Store.
//
// Steps run in a fixed order with no dependency-graph machinery: tracing
// every step found no real cross-step dependency. The one genuine
// intra-step ordering requirement — seed the admin account, or recover one
// left locked out, never both — is handled by Store.SeedAdmin's own atomic
// closure, unaffected by where Run calls it from.
func Run(ctx context.Context, store *appconfigstore.Store) (Report, error) {
	var report Report

	apiKeysTemplate, err := appconfig.BuiltinAPIKeysTemplateJSON()
	if err != nil {
		return report, fmt.Errorf("bootstrap: loading api_keys defaults: %w", err)
	}

	var apiKeysSeeded bool
	if err := store.Bootstrap(ctx, apiKeysSeedStep(apiKeysTemplate, &apiKeysSeeded)); err != nil {
		return report, fmt.Errorf("bootstrap step %q: %w", "api_keys_seed", err)
	}

	adminSeed, err := store.SeedAdmin(ctx)
	if err != nil {
		return report, fmt.Errorf("bootstrap step %q: %w", "admin_seed", err)
	}
	report.Admin = adminSeed

	plainSteps := []struct {
		name  string
		apply func(*appconfig.Config) (bool, error)
	}{
		{"directory_scanner_defaults", directoryScannerDefaultsStep},
		{"sidecar_types_seed", sidecarTypesSeedStep},
		{"agents_normalize", agentsNormalizeStep},
		{"logging_defaults", loggingDefaultsStep},
	}
	for _, step := range plainSteps {
		if err := store.Bootstrap(ctx, step.apply); err != nil {
			return report, fmt.Errorf("bootstrap step %q: %w", step.name, err)
		}
	}

	var sidecarTypesAdded int
	if err := store.Bootstrap(ctx, sidecarTypesMergeMissingStep(&sidecarTypesAdded)); err != nil {
		return report, fmt.Errorf("bootstrap step %q: %w", "sidecar_types_merge_missing", err)
	}
	report.SidecarTypesAdded = sidecarTypesAdded

	var apiKeyIDsMinted int
	if err := store.Bootstrap(ctx, apiKeyIDsBackfillStep(&apiKeyIDsMinted)); err != nil {
		return report, fmt.Errorf("bootstrap step %q: %w", "api_key_ids_backfill", err)
	}
	report.APIKeyIDsBackfilled = apiKeyIDsMinted

	if apiKeysSeeded {
		final, err := store.Read(ctx)
		if err != nil {
			return report, err
		}
		report.APIKeys = &final.APIKeys
	}

	return report, nil
}
