package appconfig

import (
	"testing"
	"time"

	"Metarr/internal/shared/eventbus"
)

// The event_bus built-in defaults are the single source of truth an operator
// starts from; eventbus.DefaultRetryPolicy / DefaultRetentionPolicy are what
// a process with no live config (the agent) runs with. They must not drift:
// this pins builtin_defaults.json to those policies rather than restating
// the numbers by hand.
func TestEventBusBuiltinDefaultsMatchThePackagePolicies(t *testing.T) {
	cfg := Default().EventBus
	if cfg == nil {
		t.Fatal("Default().EventBus is nil")
	}

	retention := eventbus.DefaultRetentionPolicy()
	if int64(cfg.GetMaxLen()) != retention.MaxLen {
		t.Errorf("max_len = %d, want %d", cfg.GetMaxLen(), retention.MaxLen)
	}
	if int(cfg.GetRetentionHours()) != retention.RetentionHours {
		t.Errorf("retention_hours = %d, want %d", cfg.GetRetentionHours(), retention.RetentionHours)
	}

	retry := eventbus.DefaultRetryPolicy()
	if int(cfg.GetRetryAttempts()) != retry.MaxAttempts {
		t.Errorf("retry_attempts = %d, want %d", cfg.GetRetryAttempts(), retry.MaxAttempts)
	}
	if time.Duration(cfg.GetRetryBackoffBaseMs())*time.Millisecond != retry.BackoffBase {
		t.Errorf("retry_backoff_base_ms = %d, want %d", cfg.GetRetryBackoffBaseMs(), retry.BackoffBase.Milliseconds())
	}
	if time.Duration(cfg.GetRetryBackoffMaxMs())*time.Millisecond != retry.BackoffMax {
		t.Errorf("retry_backoff_max_ms = %d, want %d", cfg.GetRetryBackoffMaxMs(), retry.BackoffMax.Milliseconds())
	}
}

// The live-config converters must round-trip the section they are handed.
func TestEventBusPolicyConvertersReadTheLiveSection(t *testing.T) {
	cfg := &EventBusConfig{
		MaxLen: 2, RetentionHours: 12,
		RetryAttempts: 7, RetryBackoffBaseMs: 250, RetryBackoffMaxMs: 9000,
	}

	retention := eventbus.RetentionPolicyFromConfig(cfg)
	if retention.MaxLen != 2 || retention.RetentionHours != 12 {
		t.Errorf("RetentionPolicyFromConfig = %+v", retention)
	}

	retry := eventbus.RetryPolicyFromConfig(cfg)
	if retry.MaxAttempts != 7 || retry.BackoffBase != 250*time.Millisecond || retry.BackoffMax != 9*time.Second {
		t.Errorf("RetryPolicyFromConfig = %+v", retry)
	}
}
