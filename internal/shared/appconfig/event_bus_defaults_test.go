package appconfig

import (
	"reflect"
	"testing"
	"time"

	"Metarr/internal/shared/eventbus"
)

// The event_bus built-in defaults are the single source of truth an operator
// starts from; eventbus.DefaultBusPolicy is what a process with no live
// config (the agent) runs with. They must not drift: this pins
// builtin_defaults.json, run through the one config adapter, to
// DefaultBusPolicy rather than restating the numbers by hand.
func TestEventBusBuiltinDefaultsMatchThePackagePolicy(t *testing.T) {
	cfg := Default().EventBus
	if cfg == nil {
		t.Fatal("Default().EventBus is nil")
	}

	got := eventbus.BusPolicyFromConfig(cfg)
	want := eventbus.DefaultBusPolicy()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BusPolicyFromConfig(builtin defaults) = %+v, want DefaultBusPolicy() %+v", got, want)
	}
}

// The live-config adapter must round-trip the section it is handed.
func TestBusPolicyFromConfigReadsTheLiveSection(t *testing.T) {
	cfg := &EventBusConfig{
		MaxLen: 2, RetentionHours: 12,
		RetryAttempts: 7, RetryBackoffBaseMs: 250, RetryBackoffMaxMs: 9000,
	}

	policy := eventbus.BusPolicyFromConfig(cfg)
	if policy.Retention.MaxLen != 2 || policy.Retention.RetentionHours != 12 {
		t.Errorf("Retention = %+v", policy.Retention)
	}
	if policy.Retry.MaxAttempts != 7 ||
		policy.Retry.BackoffBase != 250*time.Millisecond ||
		policy.Retry.BackoffMax != 9*time.Second {
		t.Errorf("Retry = %+v", policy.Retry)
	}
	// The sweep interval is not a live config field; it stays the compiled default.
	if policy.SweepInterval != eventbus.DefaultSweepInterval {
		t.Errorf("SweepInterval = %s, want the compiled default %s", policy.SweepInterval, eventbus.DefaultSweepInterval)
	}
}
