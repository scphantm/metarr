package config

// Agent configuration is deliberately tiny and separate from the server's.
//
// Everything the agent actually does — which libraries it can see, how they
// map onto its own paths, how the scanner is tuned — arrives over Redis. What
// has to be on the remote machine is only what it takes to find Redis and to
// say who you are. That is the whole point of the split: deploying an agent
// should not mean copying the server's configuration onto someone else's box.
//
// The file is optional. A container can be configured entirely by environment
// variables, and requiring a mounted YAML file to set two values would be a
// poor trade.

import (
	"fmt"
	"net/url"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

const (
	defaultAgentConfigPath  = "agent.yaml"
	agentConfigFileEnvVar   = "METARR_AGENT_CONFIG_FILE"
	defaultAgentLogFilePath = "logs/agent.log"
	defaultRedisPort        = 6379
)

// AgentConfig is everything metarr-agent is configured with locally.
type AgentConfig struct {
	// Slug is this agent's name. It is how the server addresses work to this
	// machine and how the operator recognises it in the UI, so it has to be
	// stable across restarts and unique across agents.
	Slug string

	RedisURI    string
	LogFilePath string
}

type agentFileConfig struct {
	Agent struct {
		Slug string `yaml:"slug"`
	} `yaml:"agent"`
	Redis struct {
		// URI wins when set. The discrete fields below exist because that is
		// how an operator thinks about a remote connection, and because
		// putting a password in a URI means remembering to percent-encode it.
		URI      string `yaml:"uri"`
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
	} `yaml:"redis"`
	Logging struct {
		File string `yaml:"file"`
	} `yaml:"logging"`
}

// LoadAgent reads the agent's configuration from agent.yaml (or the file named
// by METARR_AGENT_CONFIG_FILE) and applies environment overrides on top.
//
// A missing file is not an error as long as the environment supplies a slug
// and a way to reach Redis.
func LoadAgent() (AgentConfig, error) {
	path := defaultAgentConfigPath
	if override := os.Getenv(agentConfigFileEnvVar); override != "" {
		path = override
	}

	var parsed agentFileConfig
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return AgentConfig{}, fmt.Errorf("config: parsing %s: %w", path, err)
		}
	case os.IsNotExist(err):
		// Environment-only configuration is a supported deployment, so this
		// is only fatal if the environment turns out not to supply the
		// required values either — reported below, where the message can say
		// which value is actually missing.
	default:
		return AgentConfig{}, fmt.Errorf("config: reading %s: %w", path, err)
	}

	applyEnv(&parsed)

	slug := parsed.Agent.Slug
	if slug == "" {
		return AgentConfig{}, fmt.Errorf(
			"config: an agent slug is required; set agent.slug in %s or METARR_AGENT_SLUG",
			path,
		)
	}

	redisURI, err := parsed.redisURI()
	if err != nil {
		return AgentConfig{}, fmt.Errorf("config: %s: %w", path, err)
	}

	logFilePath := parsed.Logging.File
	if logFilePath == "" {
		logFilePath = defaultAgentLogFilePath
	}

	return AgentConfig{
		Slug:        slug,
		RedisURI:    redisURI,
		LogFilePath: logFilePath,
	}, nil
}

func applyEnv(parsed *agentFileConfig) {
	if value := os.Getenv("METARR_AGENT_SLUG"); value != "" {
		parsed.Agent.Slug = value
	}
	if value := os.Getenv("METARR_REDIS_URI"); value != "" {
		parsed.Redis.URI = value
	}
	if value := os.Getenv("METARR_REDIS_HOST"); value != "" {
		parsed.Redis.Host = value
	}
	if value := os.Getenv("METARR_REDIS_PORT"); value != "" {
		if port, err := strconv.Atoi(value); err == nil {
			parsed.Redis.Port = port
		}
	}
	if value := os.Getenv("METARR_REDIS_USERNAME"); value != "" {
		parsed.Redis.Username = value
	}
	if value := os.Getenv("METARR_REDIS_PASSWORD"); value != "" {
		parsed.Redis.Password = value
	}
	if value := os.Getenv("METARR_REDIS_DB"); value != "" {
		if db, err := strconv.Atoi(value); err == nil {
			parsed.Redis.DB = db
		}
	}
	if value := os.Getenv("METARR_AGENT_LOG_FILE"); value != "" {
		parsed.Logging.File = value
	}
}

// redisURI returns the connection string, either as given or assembled from
// the discrete fields. Credentials are percent-encoded on the way in, so a
// password containing @ or / does not silently produce a URI pointing
// somewhere else.
func (c agentFileConfig) redisURI() (string, error) {
	if c.Redis.URI != "" {
		return c.Redis.URI, nil
	}

	if c.Redis.Host == "" {
		return "", fmt.Errorf(
			"a Redis connection is required; set redis.uri or redis.host " +
				"(or METARR_REDIS_URI / METARR_REDIS_HOST)",
		)
	}

	port := c.Redis.Port
	if port == 0 {
		port = defaultRedisPort
	}

	redisURL := url.URL{
		Scheme: "redis",
		Host:   fmt.Sprintf("%s:%d", c.Redis.Host, port),
		Path:   "/" + strconv.Itoa(c.Redis.DB),
	}
	if c.Redis.Username != "" || c.Redis.Password != "" {
		redisURL.User = url.UserPassword(c.Redis.Username, c.Redis.Password)
	}

	return redisURL.String(), nil
}
