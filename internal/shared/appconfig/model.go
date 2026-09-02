// Package appconfig models the application's mutable runtime configuration
// (as exampled by app_config.local.yaml) so it can be stored as a single
// document in MongoDB, served over the API, and kept available in-memory as
// a process-wide singleton once loaded.
//
// The model types are aliases to their generated metarr.v1 messages: proto
// is the single definition for anything that crosses a language boundary,
// and the config document crosses three (Go server, TypeScript UI, stored
// document). There is no hand-written mirror and no conversion layer — the
// type the service layer receives is the type the store persists. See
// docs/adr/0005.
package appconfig

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	busv1 "Metarr/internal/genproto/metarr/bus/v1"
	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// SingletonID is the fixed _id every Config document is stored and queried
// under, guaranteeing there is ever only one copy of this document in the
// database regardless of how many times it's replaced. It is a storage
// concern, not a setting, so it is not a field on the message.
const SingletonID = "app_config"

// The application config model. Every type here is an alias to the generated
// message that defines it — see the package doc.
type (
	Config                 = metarrv1.Config
	AdminUser              = metarrv1.AdminUser
	APIKeysConfig          = metarrv1.APIKeysConfig
	APIKeyEntry            = metarrv1.APIKeyEntry
	InterfacesConfig       = metarrv1.InterfacesConfig
	SonarrInstance         = metarrv1.SonarrInstance
	RootDirMapping         = metarrv1.RootDirMapping
	StorageConfig          = metarrv1.StorageConfig
	DirectoryScannerConfig = metarrv1.DirectoryScannerConfig
	ScanDirectory          = metarrv1.ScanDirectory
	SidecarTypeDefinition  = busv1.SidecarTypeDefinition
	AgentConfig            = metarrv1.AgentConfig
	AgentDirectoryMapping  = metarrv1.AgentDirectoryMapping
	LoggingConfig          = metarrv1.LoggingConfig
	EventBusConfig         = metarrv1.EventBusConfig
)

// LogLevelInfo and LogLevelDebug are the two levels the System > Logging
// screen switches between. Warn and Error always come through regardless of
// this setting — it is a verbosity floor, not a filter on severity.
const (
	LogLevelInfo  = "info"
	LogLevelDebug = "debug"
)

// storedMarshal encodes the config document the way it is both stored and
// transported: proto field names, so the stored field names stay snake_case
// and the document is readable directly in the database, and every field
// emitted, so the document lists every setting rather than only the ones
// that differ from zero.
var storedMarshal = protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}

// storedUnmarshal is the matching decoder. DiscardUnknown keeps a document
// written by a newer build loadable by an older one.
var storedUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

// MarshalStored encodes cfg as the canonical stored/wire JSON form. It is
// the one place the config document's serialization is defined, shared by
// the Mongo repo and the config-update event payload. The config API carries
// no derived fields (ADR-0005), so cfg is marshalled as-is.
func MarshalStored(cfg *Config) ([]byte, error) {
	return storedMarshal.Marshal(cfg)
}

// UnmarshalStored decodes bytes produced by MarshalStored (or any protojson
// encoding of a Config) back into a Config. It does not Normalize — callers
// that need every section filled do that themselves, at the point the
// config enters the process.
func UnmarshalStored(data []byte) (*Config, error) {
	var cfg Config
	if err := storedUnmarshal.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Normalize fills in every section of config that is nil, and returns it.
//
// The sections are pointers, so a document that has never carried one — the
// case startup bootstrap exists to handle — decodes with that section nil
// rather than zeroed. Filling them once, where a config enters the process,
// is what lets every read site use plain field access instead of guarding
// each one; there are hundreds of read sites and one entry point.
//
// It only fills nils. A section that decoded with contents is left exactly
// as it is, including one that is deliberately empty, so this can never be
// mistaken for applying defaults — that is bootstrap's job and a separate
// decision (see docs/adr/0004).
//
// The body is a sequence of independent, single-purpose blocks so that a
// later change can add one without touching or reordering the rest.
func Normalize(config *Config) *Config {
	if config == nil {
		return Default()
	}

	normalizeSections(config)
	normalizeSonarrStorage(config)

	return config
}

// normalizeSections fills every top-level config section that decoded nil,
// so every read site can use plain field access. A document that has never
// carried a section — the case startup bootstrap exists to handle — decodes
// with it nil rather than zeroed.
func normalizeSections(config *Config) {
	if config.ApiKeys == nil {
		config.ApiKeys = &APIKeysConfig{}
	}
	if config.Admin == nil {
		config.Admin = &AdminUser{}
	}
	if config.Interfaces == nil {
		config.Interfaces = &InterfacesConfig{}
	}
	if config.DirectoryScanner == nil {
		config.DirectoryScanner = &DirectoryScannerConfig{}
	}
	if config.Logging == nil {
		config.Logging = &LoggingConfig{}
	}
	if config.EventBus == nil {
		config.EventBus = &EventBusConfig{}
	}
}

// normalizeSonarrStorage gives every Sonarr instance a storage section: one
// with none would fail the first time the cache consulted its mode. A null
// entry in the array decodes to a nil instance, which is a malformed
// document rather than a shape to repair — it is skipped so that reading the
// config reports the problem elsewhere instead of panicking during startup.
func normalizeSonarrStorage(config *Config) {
	for _, instance := range config.Interfaces.Sonarr {
		if instance == nil {
			continue
		}
		if instance.Storage == nil {
			instance.Storage = &StorageConfig{}
		}
	}
}

// FindAgentIndex returns the index of the agent entry with the given slug, or
// -1 if none matches.
func FindAgentIndex(config *Config, slug string) int {
	for i, agent := range config.Agents {
		if agent.Slug == slug {
			return i
		}
	}
	return -1
}

// AgentForScanner returns the agent mapped to scannerSlug.
//
// Only one agent may own a scan directory: two agents mapping the same library
// would both scan it and each overwrite the other's records with its own view.
// The API refuses the second mapping, so finding the first is finding the only.
func AgentForScanner(config *Config, scannerSlug string) (*AgentConfig, bool) {
	for _, agent := range config.Agents {
		for _, mapping := range agent.Mappings {
			if mapping.ScannerSlug == scannerSlug {
				return agent, true
			}
		}
	}
	return nil, false
}

// FindMapping returns agent's mapping for scannerSlug.
func FindMapping(agent *AgentConfig, scannerSlug string) (*AgentDirectoryMapping, bool) {
	for _, mapping := range agent.Mappings {
		if mapping.ScannerSlug == scannerSlug {
			return mapping, true
		}
	}
	return nil, false
}

// AllInstanceSlugs returns every instance_slug currently in use across all
// interface types. instance_slug must be unique across all interfaces, not
// just within one type, so this is the one place to extend when a new
// interface type (e.g. Radarr) is added alongside Sonarr.
func AllInstanceSlugs(interfaces *InterfacesConfig) []string {
	slugs := make([]string, 0, len(interfaces.Sonarr))
	for _, instance := range interfaces.Sonarr {
		slugs = append(slugs, instance.InstanceSlug)
	}
	return slugs
}

// FindSonarrIndex returns the index of the Sonarr instance with the given
// slug, or -1 if none matches.
func FindSonarrIndex(interfaces *InterfacesConfig, slug string) int {
	for i, instance := range interfaces.Sonarr {
		if instance.InstanceSlug == slug {
			return i
		}
	}
	return -1
}

// FindScanDirectoryIndex returns the index of the scan directory entry with
// the given scanner_slug, or -1 if none matches.
func FindScanDirectoryIndex(scanner *DirectoryScannerConfig, slug string) int {
	for i, entry := range scanner.ScanDirectories {
		if entry.ScannerSlug == slug {
			return i
		}
	}
	return -1
}

// FindSidecarTypeIndexByID returns the index of the sidecar type entry with the
// given id, or -1 if none matches. Sidecar types are keyed on a minted id rather
// than on their type name, so an entry can be renamed without the API losing
// track of it.
func FindSidecarTypeIndexByID(scanner *DirectoryScannerConfig, id string) int {
	for i, entry := range scanner.SidecarTypes {
		if entry.Id == id {
			return i
		}
	}
	return -1
}

// Default returns the zero-value configuration (matching
// app_config.default.yaml: no API keys or interfaces configured yet).
//
// Static sections (directory scanner settings, sidecar types, logging) come
// from the same embedded builtin_defaults.json the startup bootstrap reads,
// so the two can never again drift the way ParallelCount once did — see
// docs/adr/0004-bootstrap-module-and-embedded-defaults-file.md. API keys and
// the admin account stay empty here regardless: those are generated, not
// defaulted, and generating them is the startup bootstrap's job, not this
// function's.
func Default() *Config {
	defaults := loadBuiltinDefaults()
	return &Config{
		ApiKeys: &APIKeysConfig{
			Admin:    []*APIKeyEntry{},
			User:     []*APIKeyEntry{},
			Webhook:  []*APIKeyEntry{},
			ReadOnly: []*APIKeyEntry{},
		},
		Admin:      &AdminUser{},
		Interfaces: &InterfacesConfig{Sonarr: []*SonarrInstance{}},
		DirectoryScanner: &DirectoryScannerConfig{
			ParallelCount:   defaults.DirectoryScanner.ParallelCount,
			ScanDirectories: []*ScanDirectory{},
			SidecarTypes:    DefaultSidecarTypes(),
		},
		Agents: []*AgentConfig{},
		// Cloned: loadBuiltinDefaults caches its parse, and a caller that
		// mutates cfg.Logging or cfg.EventBus must not reach through into
		// that shared copy.
		Logging:  proto.Clone(defaults.Logging).(*LoggingConfig),
		EventBus: proto.Clone(defaults.EventBus).(*EventBusConfig),
	}
}
