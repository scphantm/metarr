package eventbus

import (
	"testing"
	"time"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// DefaultBusPolicy is the sole public default: it must return the documented
// retention floor, retry tuning, and sweep interval.
func TestDefaultBusPolicyReturnsTheDocumentedValues(t *testing.T) {
	policy := DefaultBusPolicy()

	if policy.Retention.MaxLen != DefaultMaxLen {
		t.Errorf("Retention.MaxLen = %d, want %d", policy.Retention.MaxLen, DefaultMaxLen)
	}
	if policy.Retention.RetentionHours != DefaultRetentionHours {
		t.Errorf("Retention.RetentionHours = %d, want %d", policy.Retention.RetentionHours, DefaultRetentionHours)
	}
	if policy.Retry.MaxAttempts != DefaultRetryAttempts {
		t.Errorf("Retry.MaxAttempts = %d, want %d", policy.Retry.MaxAttempts, DefaultRetryAttempts)
	}
	if policy.Retry.BackoffBase != DefaultRetryBackoffBase {
		t.Errorf("Retry.BackoffBase = %s, want %s", policy.Retry.BackoffBase, DefaultRetryBackoffBase)
	}
	if policy.Retry.BackoffMax != DefaultRetryBackoffMax {
		t.Errorf("Retry.BackoffMax = %s, want %s", policy.Retry.BackoffMax, DefaultRetryBackoffMax)
	}
	if policy.SweepInterval != DefaultSweepInterval {
		t.Errorf("SweepInterval = %s, want %s", policy.SweepInterval, DefaultSweepInterval)
	}
}

// BusPolicyFromConfig maps an EventBusConfig onto the same BusPolicy shape,
// leaving the sweep interval at the compiled default (it is not a config
// field).
func TestBusPolicyFromConfigMapsEveryField(t *testing.T) {
	cfg := &metarrv1.EventBusConfig{
		MaxLen:             3,
		RetentionHours:     6,
		RetryAttempts:      9,
		RetryBackoffBaseMs: 125,
		RetryBackoffMaxMs:  12000,
	}

	policy := BusPolicyFromConfig(cfg)

	want := BusPolicy{
		Retention:     RetentionPolicy{MaxLen: 3, RetentionHours: 6},
		Retry:         RetryPolicy{MaxAttempts: 9, BackoffBase: 125 * time.Millisecond, BackoffMax: 12 * time.Second},
		SweepInterval: DefaultSweepInterval,
	}
	if policy != want {
		t.Errorf("BusPolicyFromConfig = %+v, want %+v", policy, want)
	}
}
