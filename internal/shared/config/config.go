// Package config loads runtime configuration for the API from a YAML file
// at the project root (config.yaml by default), so it shares a single
// source of connection info with the rest of the project. The file path
// can be overridden with the METARR_CONFIG_FILE environment variable.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigPath = "config.yaml"
	configFileEnvVar  = "METARR_CONFIG_FILE"

	defaultHeartbeatTimeout = 5 * time.Second
)

// Config is the application's runtime configuration, loaded from the
// project's config.yaml (or the file named by METARR_CONFIG_FILE).
type Config struct {
	// HTTP server
	Host string
	Port int

	// MongoDB
	MongoURI      string
	MongoDatabase string

	// Redis
	RedisURI string

	// LogForwardURL is where the server forwards every log record it sees on
	// eventbus.LogChannel — Fluent Bit's HTTP input, e.g.
	// "http://fluent-bit:8888/app_logs". Optional: a deployment with no
	// Fluent Bit simply doesn't set it, and the server logs to stdout and the
	// Redis channel only, same as an agent does. This is infrastructure
	// wiring (which host, which port), not application config, which is why
	// it lives here rather than in the Mongo-stored appconfig.LoggingConfig —
	// the same reasoning that keeps MongoURI/RedisURI out of that document.
	LogForwardURL string

	// WorkflowCatalogPath is the hand-edited node type catalog the server
	// reads at startup and serves to the editor. Like LogForwardURL this is
	// infrastructure wiring — where the file lives on disk — rather than an
	// application setting, so it belongs here rather than in the
	// Mongo-stored appconfig.
	WorkflowCatalogPath string

	// HeartbeatTimeout bounds how long the blocking heartbeat call will
	// wait for the listener's reply before failing the request.
	HeartbeatTimeout time.Duration
}

// fileConfig mirrors the shared config.yaml schema.
type fileConfig struct {
	MongoDB struct {
		AppURI   string `yaml:"app_uri"`
		Database string `yaml:"database"`
	} `yaml:"mongodb"`
	Redis struct {
		URI string `yaml:"uri"`
	} `yaml:"redis"`
	Server struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
	Logging struct {
		ForwardURL string `yaml:"forward_url"`
	} `yaml:"logging"`
	Workflow struct {
		CatalogPath string `yaml:"catalog_path"`
	} `yaml:"workflow"`
}

// defaultWorkflowCatalogPath is where the catalog lives when the config file
// does not say otherwise — the repo root, alongside config.yaml.
const defaultWorkflowCatalogPath = "catalog.json"

// Load reads and validates the config file, failing fast with a
// path-naming error if it's missing, unparseable, or missing required
// fields.
func Load() (Config, error) {
	path := defaultConfigPath
	if override := os.Getenv(configFileEnvVar); override != "" {
		path = override
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: reading %s: %w", path, err)
	}

	var parsedConfig fileConfig
	if err := yaml.Unmarshal(data, &parsedConfig); err != nil {
		return Config{}, fmt.Errorf("config: parsing %s: %w", path, err)
	}

	if parsedConfig.MongoDB.AppURI == "" {
		return Config{}, fmt.Errorf("config: %s: mongodb.app_uri is required", path)
	}
	if parsedConfig.MongoDB.Database == "" {
		return Config{}, fmt.Errorf("config: %s: mongodb.database is required", path)
	}
	if parsedConfig.Redis.URI == "" {
		return Config{}, fmt.Errorf("config: %s: redis.uri is required", path)
	}

	catalogPath := parsedConfig.Workflow.CatalogPath
	if catalogPath == "" {
		catalogPath = defaultWorkflowCatalogPath
	}

	return Config{
		Host:                parsedConfig.Server.Host,
		Port:                parsedConfig.Server.Port,
		MongoURI:            parsedConfig.MongoDB.AppURI,
		MongoDatabase:       parsedConfig.MongoDB.Database,
		RedisURI:            parsedConfig.Redis.URI,
		LogForwardURL:       parsedConfig.Logging.ForwardURL,
		WorkflowCatalogPath: catalogPath,
		HeartbeatTimeout:    defaultHeartbeatTimeout,
	}, nil
}
