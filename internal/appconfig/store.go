package appconfig

import "sync/atomic"

// current is the process-wide singleton holding the live application
// config. It's read lock-free by anything that needs config values, and
// swapped atomically by the system_config_update listener whenever the
// config changes.
var current atomic.Pointer[Config]

func init() {
	current.Store(Default())
}

// Get returns the current in-memory application config.
func Get() *Config {
	return current.Load()
}

// Set replaces the in-memory singleton with cfg.
func Set(cfg *Config) {
	current.Store(cfg)
}
