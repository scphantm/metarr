package appconfigstore

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"Metarr/internal/server/passwordhash"
	"Metarr/internal/shared/appconfig"
)

func TestRecoverLockedOutAdmin_FreshInstallIsUntouched(t *testing.T) {
	admin := &appconfig.AdminUser{}

	password, recovered, err := recoverLockedOutAdmin(admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recovered {
		t.Fatal("a record with no username should not be treated as locked out")
	}
	if password != "" {
		t.Fatalf("expected no password to be generated, got %q", password)
	}
	if !proto.Equal(admin, &appconfig.AdminUser{}) {
		t.Fatalf("record was mutated: %+v", admin)
	}
}

func TestRecoverLockedOutAdmin_IntactCredentialsAreUntouched(t *testing.T) {
	admin := &appconfig.AdminUser{
		Username:     "admin",
		Email:        "admin@example.com",
		PasswordSalt: "existing-salt",
		PasswordHash: "existing-hash",
	}
	original := proto.Clone(admin).(*appconfig.AdminUser)

	password, recovered, err := recoverLockedOutAdmin(admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recovered {
		t.Fatal("a record with intact credentials should not be recovered")
	}
	if password != "" {
		t.Fatalf("expected no password to be generated, got %q", password)
	}
	if !proto.Equal(admin, original) {
		t.Fatalf("record was mutated: got %+v, want %+v", admin, original)
	}
}

func TestRecoverLockedOutAdmin_EmptyHashIsRecovered(t *testing.T) {
	admin := &appconfig.AdminUser{
		Username: "admin",
		Email:    "admin@example.com",
		// PasswordSalt/PasswordHash left empty, as a scoped write that
		// round-tripped GetConfig's redacted response would leave them.
	}

	password, recovered, err := recoverLockedOutAdmin(admin)
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
	admin := &appconfig.AdminUser{
		Username:     "admin",
		PasswordHash: "orphaned-hash-with-no-salt",
	}

	_, recovered, err := recoverLockedOutAdmin(admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !recovered {
		t.Fatal("a hash with no matching salt is unusable and must be recovered")
	}
}

func TestRecoverLockedOutAdmin_IsIdempotentOnceRecovered(t *testing.T) {
	admin := &appconfig.AdminUser{Username: "admin"}

	_, recovered, err := recoverLockedOutAdmin(admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !recovered {
		t.Fatal("expected the first call to recover the account")
	}

	afterFirstRecovery := proto.Clone(admin).(*appconfig.AdminUser)

	_, recoveredAgain, err := recoverLockedOutAdmin(admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recoveredAgain {
		t.Fatal("a second run should not regenerate a password that was just recovered")
	}
	if !proto.Equal(admin, afterFirstRecovery) {
		t.Fatalf("second run mutated an already-recovered record: %+v", admin)
	}
}

// TestSeedAdmin_RecoveryCannotRunBeforeSeeding is the regression test for
// the ordering invariant ADR 0003 moved out of main.go's statement order and
// into this one function: a fresh install (no username yet) must always be
// seeded, never "recovered" — recovery only ever applies to an account
// seeding didn't just create.
func TestSeedAdmin_RecoveryCannotRunBeforeSeeding(t *testing.T) {
	backend := &fakeBackend{}
	store := New(backend, backend)

	result, err := store.SeedAdmin(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Recovered {
		t.Fatal("a fresh install must be seeded, not recovered")
	}
	if result.Username == "" || result.Password == "" {
		t.Fatalf("expected a seeded admin, got %+v", result)
	}

	stored, err := store.Read(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.Admin.PasswordSalt == "" || stored.Admin.PasswordHash == "" {
		t.Fatalf("seeded admin was not persisted: %+v", stored.Admin)
	}
}

func TestSeedAdmin_RecoversAnAccountLockedOutByStoredState(t *testing.T) {
	backend := &fakeBackend{cfg: &appconfig.Config{
		Admin: &appconfig.AdminUser{Username: "admin", Email: "admin@example.com"},
	}}
	store := New(backend, backend)

	result, err := store.SeedAdmin(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Recovered {
		t.Fatal("expected the locked-out account to be recovered")
	}
	if result.Password == "" {
		t.Fatal("expected a plaintext password back")
	}
}

func TestSeedAdmin_NoopWhenAdminAlreadyIntact(t *testing.T) {
	backend := &fakeBackend{cfg: &appconfig.Config{
		Admin: &appconfig.AdminUser{
			Username:     "admin",
			Email:        "admin@example.com",
			PasswordSalt: "existing-salt",
			PasswordHash: "existing-hash",
		},
	}}
	store := New(backend, backend)

	result, err := store.SeedAdmin(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Password != "" || result.Recovered {
		t.Fatalf("expected no change for an already-intact admin, got %+v", result)
	}
	if backend.upsertCalls != 0 {
		t.Fatalf("expected no write when nothing changed, got %d upserts", backend.upsertCalls)
	}
}
