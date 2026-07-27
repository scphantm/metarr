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
	ID         string           `bson:"_id" json:"-"`
	APIKeys    APIKeysConfig    `bson:"api_keys" json:"api_keys"`
	Interfaces InterfacesConfig `bson:"interfaces" json:"interfaces"`
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
	}
}
