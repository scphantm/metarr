package config

import (
	"os"
	"path/filepath"
	"testing"
)

// repoRoot walks up from the test's working directory to the directory
// holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from test dir")
		}
		dir = parent
	}
}

// clearConfigEnv removes every environment variable that Load / LoadAgent
// consult, so the tests exercise the default file paths.
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		configFileEnvVar, agentConfigFileEnvVar,
		"METARR_AGENT_SLUG", "METARR_REDIS_URI", "METARR_REDIS_HOST",
		"METARR_REDIS_PORT", "METARR_REDIS_USERNAME", "METARR_REDIS_PASSWORD",
		"METARR_REDIS_DB",
	} {
		if orig, ok := os.LookupEnv(key); ok {
			t.Setenv(key, orig) // registers cleanup to restore
			_ = os.Unsetenv(key)
		}
	}
}

// TestLoadDefaultPaths locks the default config locations to real, parseable
// files: Load and LoadAgent must succeed from the repo root with no env
// overrides, which is what `make run` and the IDE launch configs rely on
// after the move to config/.
func TestLoadDefaultPaths(t *testing.T) {
	clearConfigEnv(t)
	t.Chdir(repoRoot(t))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() from repo root: %v", err)
	}
	if cfg.MongoURI == "" || cfg.RedisURI == "" {
		t.Fatalf("Load() returned empty connection info: %+v", cfg)
	}
	if _, err := os.Stat(cfg.WorkflowCatalogPath); err != nil {
		t.Fatalf("workflow catalog %q not found: %v", cfg.WorkflowCatalogPath, err)
	}

	agentCfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("LoadAgent() from repo root: %v", err)
	}
	if agentCfg.Slug == "" || agentCfg.RedisURI == "" {
		t.Fatalf("LoadAgent() returned incomplete config: %+v", agentCfg)
	}
}
