package main

import (
	"testing"

	"Metarr/internal/server/passwordhash"
	"Metarr/internal/shared/appconfig"
)

func TestRecoverLockedOutAdmin_FreshInstallIsUntouched(t *testing.T) {
	admin := appconfig.AdminUser{}

	password, recovered, err := recoverLockedOutAdmin(&admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recovered {
		t.Fatal("a record with no username should not be treated as locked out")
	}
	if password != "" {
		t.Fatalf("expected no password to be generated, got %q", password)
	}
	if admin != (appconfig.AdminUser{}) {
		t.Fatalf("record was mutated: %+v", admin)
	}
}

func TestRecoverLockedOutAdmin_IntactCredentialsAreUntouched(t *testing.T) {
	admin := appconfig.AdminUser{
		Username:     "admin",
		Email:        "admin@example.com",
		PasswordSalt: "existing-salt",
		PasswordHash: "existing-hash",
	}
	original := admin

	password, recovered, err := recoverLockedOutAdmin(&admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recovered {
		t.Fatal("a record with intact credentials should not be recovered")
	}
	if password != "" {
		t.Fatalf("expected no password to be generated, got %q", password)
	}
	if admin != original {
		t.Fatalf("record was mutated: got %+v, want %+v", admin, original)
	}
}

func TestRecoverLockedOutAdmin_EmptyHashIsRecovered(t *testing.T) {
	admin := appconfig.AdminUser{
		Username: "admin",
		Email:    "admin@example.com",
		// PasswordSalt/PasswordHash left empty, as a scoped write that
		// round-tripped GetConfig's redacted response would leave them.
	}

	password, recovered, err := recoverLockedOutAdmin(&admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !recovered {
		t.Fatal("expected an empty password hash to be recovered")
	}
	if password == "" {
		t.Fatal("expected a plaintext password to be returned")
	}
	if admin.Username != "admin" || admin.Email != "admin@example.com" {
		t.Fatalf("username/email must be preserved: %+v", admin)
	}
	if admin.PasswordSalt == "" || admin.PasswordHash == "" {
		t.Fatalf("expected both password fields to be set: %+v", admin)
	}
	if !passwordhash.Verify(password, admin.PasswordSalt, admin.PasswordHash) {
		t.Fatal("returned plaintext password does not verify against the stored hash")
	}
}

func TestRecoverLockedOutAdmin_EmptySaltAloneIsRecovered(t *testing.T) {
	admin := appconfig.AdminUser{
		Username:     "admin",
		PasswordHash: "orphaned-hash-with-no-salt",
	}

	_, recovered, err := recoverLockedOutAdmin(&admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !recovered {
		t.Fatal("a hash with no matching salt is unusable and must be recovered")
	}
}

func TestRecoverLockedOutAdmin_IsIdempotentOnceRecovered(t *testing.T) {
	admin := appconfig.AdminUser{Username: "admin"}

	_, recovered, err := recoverLockedOutAdmin(&admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !recovered {
		t.Fatal("expected the first call to recover the account")
	}

	afterFirstRecovery := admin

	_, recoveredAgain, err := recoverLockedOutAdmin(&admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recoveredAgain {
		t.Fatal("a second run should not regenerate a password that was just recovered")
	}
	if admin != afterFirstRecovery {
		t.Fatalf("second run mutated an already-recovered record: %+v", admin)
	}
}
