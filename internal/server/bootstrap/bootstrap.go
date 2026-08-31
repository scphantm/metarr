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

	// FinalConfig is the fully-seeded document as Run last saw it — the
	// same *appconfig.Config the static-config Bootstrap call below already
	// read from storage and mutated in place, handed back so a caller (only
	// main.go today) can warm the live config singleton from it directly
	// instead of paying a second Mongo round trip to re-read what Run
	// already has in hand.
	FinalConfig *appconfig.Config
}

// Run seeds every part of the application config a fresh or upgrading
// install needs before the server starts serving requests, in two Mongo
// round trips: one for the admin account (store.SeedAdmin), one for
// everything else (a single store.Bootstrap call running every other step
// in sequence against one read). It used to be one store.Bootstrap call per
// step — up to eight round trips on a fresh install — until issue #15 found
// tracing every step turned up no real cross-step dependency to justify
// that, so folding them into one call changes nothing about ordering, only
// how many round trips it costs.
//
// The one genuine intra-step ordering requirement — seed the admin account,
// or recover one left locked out, never both — is handled by
// Store.SeedAdmin's own atomic closure, unaffected by where Run calls it
// from. SeedAdmin runs first specifically so the static-config call's own
// read (immediately after) picks up whatever it just persisted, keeping
// FinalConfig complete.
func Run(ctx context.Context, store *appconfigstore.Store) (Report, error) {
	var report Report

	adminSeed, err := store.SeedAdmin(ctx)
	if err != nil {
		return report, fmt.Errorf("bootstrap step %q: %w", "admin_seed", err)
	}
	report.Admin = adminSeed

	apiKeysTemplate, err := appconfig.BuiltinAPIKeysTemplateJSON()
	if err != nil {
		return report, fmt.Errorf("bootstrap: loading api_keys defaults: %w", err)
	}

	var apiKeysSeeded bool
	apply := staticConfigSteps(apiKeysTemplate, &apiKeysSeeded, &report.SidecarTypesAdded, &report.APIKeyIDsBackfilled)

	var finalCfg *appconfig.Config
	if err := store.Bootstrap(ctx, func(cfg *appconfig.Config) (bool, error) {
		finalCfg = cfg
		return apply(cfg)
	}); err != nil {
		return report, fmt.Errorf("bootstrap: %w", err)
	}
	report.FinalConfig = finalCfg

	if apiKeysSeeded {
		report.APIKeys = finalCfg.ApiKeys
	}

	return report, nil
}

// staticConfigSteps returns the single apply function Run's one
// store.Bootstrap call for everything but the admin account runs: every
// remaining step, in the same fixed order Run always executed them in,
// against the one *appconfig.Config store.Bootstrap reads. A step that
// errors stops the sequence immediately, wrapped with its own name so a
// failure is still traceable to the step that caused it despite no longer
// being its own Bootstrap call.
func staticConfigSteps(apiKeysTemplate []byte, apiKeysSeeded *bool, sidecarTypesAdded, apiKeyIDsBackfilled *int) func(cfg *appconfig.Config) (bool, error) {
	steps := []struct {
		name  string
		apply func(*appconfig.Config) (bool, error)
	}{
		{"api_keys_seed", apiKeysSeedStep(apiKeysTemplate, apiKeysSeeded)},
		{"directory_scanner_defaults", directoryScannerDefaultsStep},
		{"sidecar_types_seed", sidecarTypesSeedStep},
		{"logging_defaults", loggingDefaultsStep},
		{"event_bus_defaults", eventBusDefaultsStep},
		{"sidecar_types_merge_missing", sidecarTypesMergeMissingStep(sidecarTypesAdded)},
		{"api_key_ids_backfill", apiKeyIDsBackfillStep(apiKeyIDsBackfilled)},
	}

	return func(cfg *appconfig.Config) (bool, error) {
		anyChanged := false
		for _, step := range steps {
			changed, err := step.apply(cfg)
			if err != nil {
				return false, fmt.Errorf("step %q: %w", step.name, err)
			}
			anyChanged = anyChanged || changed
		}
		return anyChanged, nil
	}
}
