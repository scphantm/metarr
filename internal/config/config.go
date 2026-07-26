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

	defaultLogFilePath      = "logs/app.log"
	defaultHeartbeatTimeout = 5 * time.Second
)

type Config struct {
	// HTTP server
	Host string
	Port int

	// MongoDB
	MongoURI      string
	MongoDatabase string

	// Redis
	RedisURI string

	// Logging
	LogFilePath string

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
}

func Load() (Config, error) {
	path := defaultConfigPath
	if override := os.Getenv(configFileEnvVar); override != "" {
		path = override
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: reading %s: %w", path, err)
	}

	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return Config{}, fmt.Errorf("config: parsing %s: %w", path, err)
	}

	if fc.MongoDB.AppURI == "" {
		return Config{}, fmt.Errorf("config: %s: mongodb.app_uri is required", path)
	}
	if fc.MongoDB.Database == "" {
		return Config{}, fmt.Errorf("config: %s: mongodb.database is required", path)
	}
	if fc.Redis.URI == "" {
		return Config{}, fmt.Errorf("config: %s: redis.uri is required", path)
	}

	return Config{
		Host:             fc.Server.Host,
		Port:             fc.Server.Port,
		MongoURI:         fc.MongoDB.AppURI,
		MongoDatabase:    fc.MongoDB.Database,
		RedisURI:         fc.Redis.URI,
		LogFilePath:      defaultLogFilePath,
		HeartbeatTimeout: defaultHeartbeatTimeout,
	}, nil
}
