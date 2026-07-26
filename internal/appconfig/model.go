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
	Interfaces InterfacesConfig `bson:"interfaces" json:"interfaces"`
}

type InterfacesConfig struct {
	Sonarr []SonarrInstance `bson:"sonarr" json:"sonarr"`
}

type SonarrInstance struct {
	InstanceName string           `bson:"instance_name" json:"instance_name"`
	InstanceSlug string           `bson:"instance_slug" json:"instance_slug"`
	SonarrURL    string           `bson:"sonarr_url" json:"sonarr_url"`
	SonarrAPIKey string           `bson:"sonarr_api_key" json:"sonarr_api_key"`
	RootDirMap   []RootDirMapping `bson:"root_dir_map" json:"root_dir_map"`
	Storage      StorageConfig    `bson:"storage" json:"storage"`
}

type RootDirMapping struct {
	SonarrPath string `bson:"sonarr_path" json:"sonarr_path"`
	LocalPath  string `bson:"local_path" json:"local_path"`
}

type StorageConfig struct {
	Mode string `bson:"mode" json:"mode"`
	TTL  string `bson:"ttl,omitempty" json:"ttl,omitempty"`
}

// Default returns the zero-value configuration (matching
// app_config.default.yaml: no interfaces configured yet).
func Default() *Config {
	return &Config{
		ID:         SingletonID,
		Interfaces: InterfacesConfig{Sonarr: []SonarrInstance{}},
	}
}
