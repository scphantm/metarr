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
// many directories it scans concurrently, and which directories it scans.
type DirectoryScannerConfig struct {
	ParallelCount   int             `bson:"parallel_count" json:"parallel_count"`
	ScanDirectories []ScanDirectory `bson:"scan_directories" json:"scan_directories"`
}

// ScanDirectory is a single filesystem path the directory scanner watches,
// tagged with the type of media expected under it. ScannerSlug identifies
// the entry for the API and CLI, playing the same role instance_slug plays
// for a SonarrInstance.
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
		Interfaces:       InterfacesConfig{Sonarr: []SonarrInstance{}},
		DirectoryScanner: DirectoryScannerConfig{ScanDirectories: []ScanDirectory{}},
	}
}
