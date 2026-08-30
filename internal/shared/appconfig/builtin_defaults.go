package appconfig

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// builtinDefaultsJSON is Metarr's built-in configuration defaults: what a
// fresh install starts with, and what a startup bootstrap seeds into a
// database missing a section. It versions with the release rather than
// being edited on disk — see docs/adr/0004 for why. api_keys carries
// "{guid}" placeholders it is not this package's job to resolve: minting a
// fresh secret is a startup-seeding concern, not a config-model one. See
// Metarr/internal/server/bootstrap.
//
//go:embed builtin_defaults.json
var builtinDefaultsJSON []byte

// builtinDefaultsDoc is the shape of builtin_defaults.json. Only the
// sections this package itself needs are typed here; api_keys stays raw
// (see BuiltinAPIKeysTemplateJSON).
type builtinDefaultsDoc struct {
	DirectoryScanner struct {
		ParallelCount int `json:"parallel_count"`
	} `json:"directory_scanner"`
	SidecarTypes []SidecarTypeDefinition `json:"sidecar_types"`
	Logging      LoggingConfig           `json:"logging"`
	Admin        struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"admin"`
}

var (
	builtinDefaultsOnce   sync.Once
	builtinDefaultsParsed builtinDefaultsDoc
)

// parseBuiltinDefaults unmarshals the embedded defaults file into a fresh
// builtinDefaultsDoc every call — no caching. A parse failure here means the
// binary was built with a corrupt builtin_defaults.json — a compiled-in
// invariant, not a runtime condition to recover from, so it panics rather
// than propagating an error through every caller.
func parseBuiltinDefaults() builtinDefaultsDoc {
	var doc builtinDefaultsDoc
	if err := json.Unmarshal(builtinDefaultsJSON, &doc); err != nil {
		panic("appconfig: embedded builtin_defaults.json is invalid: " + err.Error())
	}
	return doc
}

// loadBuiltinDefaults returns the parsed embedded defaults file, parsing it
// once and caching the result. Safe for repeat callers like Default and
// DefaultSidecarTypes because both deep-copy every slice-typed field before
// returning it — the cached doc's own backing arrays are never handed out.
//
// A caller that instead merges the cached doc's fields directly into a live
// document via reflection (dst.Set(src)) must not use this: reflect.Value's
// Set on a slice field copies the slice header, not its backing array, so
// the live document and every future Default()/DefaultSidecarTypes() caller
// would end up aliasing the same backing array — a later mutation through
// one would silently corrupt the other. Such a caller must call
// parseBuiltinDefaults directly instead, accepting a fresh parse per call
// (the embedded file is small; this runs at most once per process
// lifetime, in bootstrap.Run).
func loadBuiltinDefaults() builtinDefaultsDoc {
	builtinDefaultsOnce.Do(func() {
		builtinDefaultsParsed = parseBuiltinDefaults()
	})
	return builtinDefaultsParsed
}

// DefaultAdminIdentity returns the username and email a fresh admin account
// is seeded with. Password, salt, and hash are deliberately not part of
// this or the underlying file: a credential hash cannot be templated, so
// generating one stays hand-written Go (see appconfigstore.SeedAdmin).
func DefaultAdminIdentity() (username, email string) {
	d := loadBuiltinDefaults()
	return d.Admin.Username, d.Admin.Email
}

// BuiltinAPIKeysTemplateJSON returns the raw api_keys section of the
// embedded defaults file, unresolved: its "{guid}" placeholders are left
// for the caller to substitute with freshly generated values.
func BuiltinAPIKeysTemplateJSON() ([]byte, error) {
	var raw struct {
		APIKeys json.RawMessage `json:"api_keys"`
	}
	if err := json.Unmarshal(builtinDefaultsJSON, &raw); err != nil {
		return nil, err
	}
	return raw.APIKeys, nil
}
