// Package appconfig models the application's mutable runtime configuration
// (as exampled by app_config.local.yaml) so it can be stored as a single
// document in MongoDB, served over the API, and kept available in-memory as
// a process-wide singleton once loaded.
package appconfig

// SingletonID is the fixed _id every Config document is stored and queried
// under, guaranteeing there is ever only one copy of this document in the
// database regardless of how many times it's replaced.
const SingletonID = "app_config"

// Config is the root application configuration document.
type Config struct {
	ID               string                 `bson:"_id" json:"-"`
	APIKeys          APIKeysConfig          `bson:"api_keys" json:"api_keys"`
	Admin            AdminUser              `bson:"admin" json:"admin"`
	Interfaces       InterfacesConfig       `bson:"interfaces" json:"interfaces"`
	DirectoryScanner DirectoryScannerConfig `bson:"directory_scanner" json:"directory_scanner"`
	Agents           []AgentConfig          `bson:"agents" json:"agents"`
	Logging          LoggingConfig          `bson:"logging" json:"logging"`
}

// LogLevelInfo and LogLevelDebug are the two levels the System > Logging
// screen switches between. Warn and Error always come through regardless of
// this setting — it is a verbosity floor, not a filter on severity.
const (
	LogLevelInfo  = "info"
	LogLevelDebug = "debug"
)

// LoggingConfig controls the centralized logging pipeline: the server's own
// log level, and informational fields describing where Fluent Bit is
// currently configured to ship logs.
//
// Sink/Endpoint/Stream are informational only — changing them here updates
// what the "Open in OpenObserve" link on the Logging screen points at and
// what it displays, but does not reconfigure Fluent Bit. Repointing the
// pipeline at a different vendor is a Fluent Bit config change
// (fluent-bit/fluent-bit.conf), deliberately kept out of this document so
// that swapping vendors is never a Go code change. Each agent's own level
// lives on its AgentConfig entry instead, since it applies to one machine,
// not the whole system.
type LoggingConfig struct {
	ServerLevel string `bson:"server_level" json:"server_level"`

	Sink     string `bson:"sink" json:"sink"`
	Endpoint string `bson:"endpoint" json:"endpoint"`
	Stream   string `bson:"stream" json:"stream"`
}

// AgentConfig is one filesystem agent's configuration.
//
// An agent announces itself by connecting to Redis; this is what an operator
// then says about it. Until an entry exists here the agent is known but idle,
// which is a deliberate state rather than an error — nothing should start
// reading a remote filesystem because a process appeared on the network.
type AgentConfig struct {
	// Slug matches the slug the agent is configured with locally, and is the
	// only link between the two. It has to be unique, which the API enforces.
	Slug        string `bson:"slug" json:"slug"`
	DisplayName string `bson:"display_name,omitempty" json:"display_name,omitempty"`

	// Mappings say which libraries this agent can see and where. A scan
	// directory absent from this list is one the agent has no access to,
	// which is normal: agents sit on different machines holding different
	// storage.
	Mappings []AgentDirectoryMapping `bson:"mappings" json:"mappings"`

	// LogLevel is this agent's own verbosity, set from the System > Logging
	// screen. Empty means LogLevelInfo — most agents never need touching, so
	// storing a default rather than requiring every entry to set it keeps a
	// bare {slug, mappings} entry valid.
	LogLevel string `bson:"log_level,omitempty" json:"log_level,omitempty"`
}

// AgentDirectoryMapping ties one configured scan directory to the path the
// agent knows it by.
//
// The two paths name the same library on different machines: the server may
// call it /media/movies while the agent that actually holds it calls it
// /mnt/tank/movies. Records are always stored under the server's name, so the
// library reads the same however many agents scanned it.
type AgentDirectoryMapping struct {
	ScannerSlug string `bson:"scanner_slug" json:"scanner_slug"`
	AgentPath   string `bson:"agent_path" json:"agent_path"`
}

// FindAgentIndex returns the index of the agent entry with the given slug, or
// -1 if none matches.
func (c Config) FindAgentIndex(slug string) int {
	for i, agent := range c.Agents {
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
func (c Config) AgentForScanner(scannerSlug string) (AgentConfig, bool) {
	for _, agent := range c.Agents {
		for _, mapping := range agent.Mappings {
			if mapping.ScannerSlug == scannerSlug {
				return agent, true
			}
		}
	}
	return AgentConfig{}, false
}

// FindMapping returns this agent's mapping for scannerSlug.
func (a AgentConfig) FindMapping(scannerSlug string) (AgentDirectoryMapping, bool) {
	for _, mapping := range a.Mappings {
		if mapping.ScannerSlug == scannerSlug {
			return mapping, true
		}
	}
	return AgentDirectoryMapping{}, false
}

// AdminUser is the system's single administrative user account.
//
// PasswordSalt and PasswordHash use `omitempty` rather than `json:"-"`: the
// system_config_update event payload round-trips a full Config through
// JSON (see Handlers.fireConfigUpdate), so a hard "-" here would silently
// wipe the stored hash on every config update, not just ones touching
// admin credentials. Client-facing responses (GetConfig) are responsible
// for redacting these two fields to "" before encoding, which
// `omitempty` then drops from the response entirely.
type AdminUser struct {
	Username     string `bson:"username" json:"username"`
	Email        string `bson:"email" json:"email"`
	PasswordSalt string `bson:"password_salt" json:"password_salt,omitempty"`
	PasswordHash string `bson:"password_hash" json:"password_hash,omitempty"`
}

// APIKeysConfig groups the API keys issued for each access-level category.
type APIKeysConfig struct {
	Admin    []APIKeyEntry `bson:"admin" json:"admin"`
	User     []APIKeyEntry `bson:"user" json:"user"`
	Webhook  []APIKeyEntry `bson:"webhook" json:"webhook"`
	ReadOnly []APIKeyEntry `bson:"read_only" json:"read_only"`
}

// APIKeyEntry is a single named API key.
type APIKeyEntry struct {
	Name string `bson:"name" json:"name"`
	Key  string `bson:"api_key" json:"api_key"`
}

// InterfacesConfig groups the configuration for every external service
// interface Metarr integrates with.
type InterfacesConfig struct {
	Sonarr []SonarrInstance `bson:"sonarr" json:"sonarr"`
}

// AllInstanceSlugs returns every instance_slug currently in use across all
// interface types. instance_slug must be unique across all interfaces, not
// just within one type, so this is the one place to extend when a new
// interface type (e.g. Radarr) is added alongside Sonarr.
func (c InterfacesConfig) AllInstanceSlugs() []string {
	slugs := make([]string, 0, len(c.Sonarr))
	for _, instance := range c.Sonarr {
		slugs = append(slugs, instance.InstanceSlug)
	}
	return slugs
}

// FindSonarrIndex returns the index of the Sonarr instance with the given
// slug, or -1 if none matches.
func (c InterfacesConfig) FindSonarrIndex(slug string) int {
	for i, instance := range c.Sonarr {
		if instance.InstanceSlug == slug {
			return i
		}
	}
	return -1
}

// SonarrInstance configures a single Sonarr instance to cache data from.
type SonarrInstance struct {
	InstanceName string           `bson:"instance_name" json:"instance_name"`
	InstanceSlug string           `bson:"instance_slug" json:"instance_slug"`
	SonarrURL    string           `bson:"sonarr_url" json:"sonarr_url"`
	SonarrAPIKey string           `bson:"sonarr_api_key" json:"sonarr_api_key"`
	RootDirMap   []RootDirMapping `bson:"root_dir_map" json:"root_dir_map"`
	Storage      StorageConfig    `bson:"storage" json:"storage"`
}

// RootDirMapping maps a root folder path as Sonarr sees it to the
// corresponding local filesystem path.
type RootDirMapping struct {
	SonarrPath string `bson:"sonarr_path" json:"sonarr_path"`
	LocalPath  string `bson:"local_path" json:"local_path"`
}

// StorageConfig controls how cached data for an interface is retained:
// "cache" mode expires data after TTL elapses, "versioned" mode keeps up to
// MaxCount revisions.
type StorageConfig struct {
	Mode     string `bson:"mode" json:"mode"`
	TTL      string `bson:"ttl,omitempty" json:"ttl,omitempty"`
	MaxCount int    `bson:"max_count,omitempty" json:"max_count,omitempty"`
}

// DirectoryScannerConfig controls the background filesystem scanner: how
// many directories it scans concurrently, which directories it scans, and how
// it classifies the sidecar files it finds inside them.
type DirectoryScannerConfig struct {
	ParallelCount   int                     `bson:"parallel_count" json:"parallel_count"`
	ScanDirectories []ScanDirectory         `bson:"scan_directories" json:"scan_directories"`
	SidecarTypes    []SidecarTypeDefinition `bson:"sidecar_types" json:"sidecar_types"`
}

// ScanDirectory is a single filesystem path the directory scanner watches,
// tagged with the type of media expected under it. ScannerSlug identifies
// the entry for the API, playing the same role instance_slug plays for a
// SonarrInstance.
type ScanDirectory struct {
	ScannerSlug string `bson:"scanner_slug" json:"scanner_slug"`
	ScanType    string `bson:"scan_type" json:"scan_type"`
	Directory   string `bson:"directory" json:"directory"`
}

// FindScanDirectoryIndex returns the index of the scan directory entry with
// the given scanner_slug, or -1 if none matches.
func (c DirectoryScannerConfig) FindScanDirectoryIndex(slug string) int {
	for i, entry := range c.ScanDirectories {
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
func (c DirectoryScannerConfig) FindSidecarTypeIndexByID(id string) int {
	for i, entry := range c.SidecarTypes {
		if entry.ID == id {
			return i
		}
	}
	return -1
}

// Default returns the zero-value configuration (matching
// app_config.default.yaml: no API keys or interfaces configured yet).
func Default() *Config {
	return &Config{
		ID: SingletonID,
		APIKeys: APIKeysConfig{
			Admin:    []APIKeyEntry{},
			User:     []APIKeyEntry{},
			Webhook:  []APIKeyEntry{},
			ReadOnly: []APIKeyEntry{},
		},
		Interfaces: InterfacesConfig{Sonarr: []SonarrInstance{}},
		DirectoryScanner: DirectoryScannerConfig{
			ScanDirectories: []ScanDirectory{},
			SidecarTypes:    DefaultSidecarTypes(),
		},
		Agents: []AgentConfig{},
		Logging: LoggingConfig{
			ServerLevel: LogLevelInfo,
			Sink:        "openobserve",
			Stream:      "metarr_app",
		},
	}
}
