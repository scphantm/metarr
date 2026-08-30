package auth

import (
	"testing"

	"Metarr/internal/shared/appconfig"
)

// TestResolve_IsDeterministicForAKeyInMoreThanOneGroup pins the role
// precedence Resolve applies when the same API key string appears in more
// than one group. The groups are searched in a fixed order — admin, user,
// webhook, read only — so the resolved role is the same on every call. The
// natural way to write this lookup over grouped keys is to range a map,
// whose iteration order is randomised; that nondeterminism is invisible in
// a single-run test, so this one resolves the same key many times.
func TestResolve_IsDeterministicForAKeyInMoreThanOneGroup(t *testing.T) {
	const sharedKey = "key-present-in-two-groups"

	config := &appconfig.Config{
		ApiKeys: &appconfig.APIKeysConfig{
			User:     []*appconfig.APIKeyEntry{{Id: "u", ApiKey: sharedKey}},
			ReadOnly: []*appconfig.APIKeyEntry{{Id: "r", ApiKey: sharedKey}},
		},
	}

	for i := 0; i < 128; i++ {
		role, ok := Resolve(config, sharedKey)
		if !ok {
			t.Fatalf("iteration %d: key did not resolve", i)
		}
		// User is searched before ReadOnly, so the higher-privilege group wins.
		if role != RoleUser {
			t.Fatalf("iteration %d: role = %q, want %q (group search order is not deterministic)", i, role, RoleUser)
		}
	}
}

// TestResolve_SearchesGroupsInAdminFirstOrder checks the full precedence
// chain: a key in every group resolves to admin.
func TestResolve_SearchesGroupsInAdminFirstOrder(t *testing.T) {
	const key = "in-every-group"
	config := &appconfig.Config{
		ApiKeys: &appconfig.APIKeysConfig{
			Admin:    []*appconfig.APIKeyEntry{{Id: "a", ApiKey: key}},
			User:     []*appconfig.APIKeyEntry{{Id: "u", ApiKey: key}},
			Webhook:  []*appconfig.APIKeyEntry{{Id: "w", ApiKey: key}},
			ReadOnly: []*appconfig.APIKeyEntry{{Id: "r", ApiKey: key}},
		},
	}

	role, ok := Resolve(config, key)
	if !ok || role != RoleAdmin {
		t.Fatalf("role = %q, ok = %v; want %q, true", role, ok, RoleAdmin)
	}
}
