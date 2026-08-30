package bootstrap

import (
	"encoding/json"

	"github.com/google/uuid"

	"Metarr/internal/shared/appconfig"
)

// guidPlaceholder marks a field in builtin_defaults.json that must become a
// freshly generated, install-unique value rather than a fixed default —
// currently only API key ids and secrets. Sidecar-type ids are never
// templated: MergeMissingSidecarTypes identifies a built-in by id across
// restarts, so a regenerated id would duplicate every built-in on the next
// one.
const guidPlaceholder = "{guid}"

// apiKeysSeedStep generates the four default API key categories the first
// time the app starts against a database with none configured — mirroring
// how this used to be seeded by mongo/init/init-mongo.js. seeded is set to
// true iff this call actually generated keys, since Run needs that to know
// whether to report them (an ordinary restart must not re-report keys that
// were already shown once).
func apiKeysSeedStep(template []byte, seeded *bool) func(cfg *appconfig.Config) (bool, error) {
	return func(cfg *appconfig.Config) (bool, error) {
		if len(cfg.ApiKeys.Admin) != 0 || len(cfg.ApiKeys.User) != 0 ||
			len(cfg.ApiKeys.Webhook) != 0 || len(cfg.ApiKeys.ReadOnly) != 0 {
			return false, nil
		}

		resolved, err := resolveGUIDsJSON(template)
		if err != nil {
			return false, err
		}

		var parsed struct {
			Admin    appconfig.APIKeyEntry `json:"admin"`
			User     appconfig.APIKeyEntry `json:"user"`
			Webhook  appconfig.APIKeyEntry `json:"webhook"`
			ReadOnly appconfig.APIKeyEntry `json:"read_only"`
		}
		if err := json.Unmarshal(resolved, &parsed); err != nil {
			return false, err
		}

		cfg.ApiKeys = &appconfig.APIKeysConfig{
			Admin:    []*appconfig.APIKeyEntry{&parsed.Admin},
			User:     []*appconfig.APIKeyEntry{&parsed.User},
			Webhook:  []*appconfig.APIKeyEntry{&parsed.Webhook},
			ReadOnly: []*appconfig.APIKeyEntry{&parsed.ReadOnly},
		}
		*seeded = true
		return true, nil
	}
}

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

// agentsNormalizeStep normalizes a nil Agents slice (decoded from a
// database predating this field) to []. Agents start as an empty list
// rather than being seeded with anything: an agent exists because someone
// deployed one and then said what it may see, and inventing a default would
// mean guessing at a machine that may not be there.
func agentsNormalizeStep(cfg *appconfig.Config) (bool, error) {
	if cfg.Agents != nil {
		return false, nil
	}
	cfg.Agents = []*appconfig.AgentConfig{}
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

// apiKeyIDsBackfillStep mints an id for any API key entry stored before the
// id field existed, which would otherwise be unaddressable by the scoped
// upsert/delete operations that key on it. minted receives how many were
// minted, for Run's report.
func apiKeyIDsBackfillStep(minted *int) func(cfg *appconfig.Config) (bool, error) {
	return func(cfg *appconfig.Config) (bool, error) {
		n := appconfig.BackfillAPIKeyIDs(cfg.ApiKeys)
		if n == 0 {
			return false, nil
		}
		*minted = n
		return true, nil
	}
}

// resolveGUIDsJSON parses raw as generic JSON, replaces every string value
// that is exactly guidPlaceholder with an independently generated UUID, and
// re-marshals the result. It operates on the parsed tree rather than raw
// text specifically so only a whole-value match ("{guid}", not
// "prefix{guid}suffix") is ever substituted.
func resolveGUIDsJSON(raw []byte) ([]byte, error) {
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	return json.Marshal(resolveGUIDs(generic))
}

// resolveGUIDs walks a value produced by json.Unmarshal(_, *any) — string,
// map[string]any, []any, or a scalar — substituting guidPlaceholder strings
// in place. Every occurrence gets its own freshly generated UUID: reusing
// one value across occurrences would, for example, give every seeded API
// key the same id.
func resolveGUIDs(v any) any {
	switch val := v.(type) {
	case string:
		if val == guidPlaceholder {
			return uuid.NewString()
		}
		return val
	case map[string]any:
		for k, item := range val {
			val[k] = resolveGUIDs(item)
		}
		return val
	case []any:
		for i, item := range val {
			val[i] = resolveGUIDs(item)
		}
		return val
	default:
		return val
	}
}
