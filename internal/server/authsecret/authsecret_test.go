package authsecret

import (
	"encoding/base64"
	"errors"
	"testing"

	"Metarr/internal/shared/appconfig"
)

// withLiveSecret arranges encoded as Auth.HmacSecret on the process-wide live
// config for one test, restoring the previous config afterwards. The same
// arrange/restore technique the services and httpserver packages use for the
// appconfig singleton.
func withLiveSecret(t *testing.T, encoded string) {
	t.Helper()
	previous := appconfig.Get()
	appconfig.Set(appconfig.Normalize(&appconfig.Config{
		Auth: &appconfig.AuthConfig{HmacSecret: encoded},
	}))
	t.Cleanup(func() { appconfig.Set(previous) })
}

func encodedSecret(raw string) string {
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func TestSignThenVerifyRoundTrips(t *testing.T) {
	withLiveSecret(t, encodedSecret("a-32-byte-test-signing-secret!!!"))

	token, err := Sign("alice", "admin", 3600)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	claims, err := Verify(token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if claims.Subject != "alice" || claims.Role != "admin" {
		t.Errorf("claims = %+v, want subject=alice role=admin", claims)
	}
}

// TestVerifyFailsAfterSecretRotates is the security-relevant invariant: once
// the live secret changes, a token signed under the previous one no longer
// verifies — rotation is a complete revocation. It also exercises the
// decode cache's refresh-on-change path (the old decodeHMACSecret test).
func TestVerifyFailsAfterSecretRotates(t *testing.T) {
	withLiveSecret(t, encodedSecret("first-secret-first-secret-first!"))

	staleToken, err := Sign("bob", "user", 3600)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if _, err := Verify(staleToken); err != nil {
		t.Fatalf("Verify() before rotation error = %v", err)
	}

	withLiveSecret(t, encodedSecret("second-secret-second-secret-two"))

	if _, err := Verify(staleToken); err == nil {
		t.Fatal("Verify() accepted a token signed under the rotated-away secret")
	}

	freshToken, err := Sign("bob", "user", 3600)
	if err != nil {
		t.Fatalf("Sign() after rotation error = %v", err)
	}
	if _, err := Verify(freshToken); err != nil {
		t.Errorf("Verify() of a token signed under the new secret error = %v", err)
	}
}

func TestSecretNotConfigured(t *testing.T) {
	withLiveSecret(t, "")

	if _, err := Sign("carol", "admin", 3600); !errors.Is(err, ErrSecretNotConfigured) {
		t.Errorf("Sign() error = %v, want ErrSecretNotConfigured", err)
	}
	if _, err := Verify("any.jwt.token"); !errors.Is(err, ErrSecretNotConfigured) {
		t.Errorf("Verify() error = %v, want ErrSecretNotConfigured", err)
	}
}

func TestSecretMalformed(t *testing.T) {
	withLiveSecret(t, "not valid base64 !!!")

	if _, err := Sign("dave", "admin", 3600); !errors.Is(err, ErrSecretMalformed) {
		t.Errorf("Sign() error = %v, want ErrSecretMalformed", err)
	}
	if _, err := Verify("any.jwt.token"); !errors.Is(err, ErrSecretMalformed) {
		t.Errorf("Verify() error = %v, want ErrSecretMalformed", err)
	}
}
