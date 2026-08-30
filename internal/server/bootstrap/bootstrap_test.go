package bootstrap

import (
	"context"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"

	"Metarr/internal/server/appconfigstore"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/eventbus"
)

// fakeStore plays appconfigstore.Store's three dependencies. Unlike a real
// Mongo-backed repo it never synthesizes appconfig.Default() on an empty
// read — starting it at appconfig.Config{} simulates a database predating
// every seeded field; starting it at *appconfig.Default() simulates a
// genuinely fresh install, where mongostore.AppConfigRepo.Get already
// returned the defaulted document before Run's first read.
type fakeStore struct {
	mu          sync.Mutex
	cfg         *appconfig.Config
	getCalls    int
	upsertCalls int
}

func (f *fakeStore) Get(_ context.Context) (*appconfig.Config, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	return proto.Clone(f.cfg).(*appconfig.Config), nil
}

func (f *fakeStore) Upsert(_ context.Context, cfg *appconfig.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cfg = proto.Clone(cfg).(*appconfig.Config)
	f.upsertCalls++
	return nil
}

func (f *fakeStore) Fire(_ context.Context, _ string, _ eventbus.Event) error {
	return nil
}

func newStoreOn(cfg *appconfig.Config) (*appconfigstore.Store, *fakeStore) {
	backend := &fakeStore{cfg: cfg}
	return appconfigstore.New(backend, backend, backend), backend
}

func TestRun_SeedsEverythingOnADatabasePredatingAllFields(t *testing.T) {
	store, backend := newStoreOn(&appconfig.Config{})

	report, err := Run(context.Background(), store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	final, err := store.Read(context.Background())
	if err != nil {
		t.Fatalf("unexpected error reading final config: %v", err)
	}

	if report.APIKeys == nil {
		t.Fatal("expected API keys to be reported as seeded")
	}
	for name, entries := range map[string][]*appconfig.APIKeyEntry{
		"admin": report.APIKeys.Admin, "user": report.APIKeys.User,
		"webhook": report.APIKeys.Webhook, "read_only": report.APIKeys.ReadOnly,
	} {
		if len(entries) != 1 {
			t.Fatalf("%s: expected exactly one seeded entry, got %d", name, len(entries))
		}
		if entries[0].Id == "" || entries[0].ApiKey == "" {
			t.Fatalf("%s: expected a generated id and key, got %+v", name, entries[0])
		}
	}

	if report.Admin.Username == "" || report.Admin.Password == "" {
		t.Fatalf("expected a seeded admin account, got %+v", report.Admin)
	}
	if final.Admin.PasswordSalt == "" || final.Admin.PasswordHash == "" {
		t.Fatalf("expected persisted admin credentials, got %+v", final.Admin)
	}

	if final.DirectoryScanner.ParallelCount == 0 {
		t.Fatal("expected directory scanner defaults to be seeded")
	}
	if final.Logging.ServerLevel == "" {
		t.Fatal("expected logging defaults to be seeded")
	}
	if len(final.DirectoryScanner.SidecarTypes) == 0 {
		t.Fatal("expected the sidecar type table to be seeded")
	}

	if backend.upsertCalls == 0 {
		t.Fatal("expected at least one write on a database predating every field")
	}
}

// TestRun_CostsOnlyTwoMongoRoundTrips is the regression test for issue #15's
// round-trip finding: Run used to cost up to 8 store.Bootstrap calls (one
// per step) plus a separate store.Read in main.go to warm the live config.
// It now costs exactly 2 store.Bootstrap calls — admin_seed, then one
// consolidated call for every other step — and zero extra reads, because
// main.go sources the live config from Report.FinalConfig instead. Get is
// the round-trip to watch: each store.Bootstrap call issues exactly one.
func TestRun_CostsOnlyTwoMongoRoundTrips(t *testing.T) {
	store, backend := newStoreOn(&appconfig.Config{})

	report, err := Run(context.Background(), store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if backend.getCalls != 2 {
		t.Errorf("getCalls = %d, want 2 (admin_seed + one consolidated static-config call)", backend.getCalls)
	}
	if report.FinalConfig == nil {
		t.Fatal("expected Report.FinalConfig to be populated")
	}
	if report.FinalConfig.DirectoryScanner.ParallelCount == 0 {
		t.Error("Report.FinalConfig does not reflect the static-config step's writes")
	}
	if len(report.FinalConfig.ApiKeys.Admin) != 1 {
		t.Error("Report.FinalConfig does not reflect the api_keys_seed step's writes")
	}
}

func TestRun_AgreesWithDefaultOnTheStaticSections(t *testing.T) {
	store, _ := newStoreOn(&appconfig.Config{})

	if _, err := Run(context.Background(), store); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	final, err := store.Read(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := appconfig.Default()
	if final.DirectoryScanner.ParallelCount != want.DirectoryScanner.ParallelCount {
		t.Errorf("ParallelCount = %d, want %d (appconfig.Default disagrees with Run)",
			final.DirectoryScanner.ParallelCount, want.DirectoryScanner.ParallelCount)
	}
	if !proto.Equal(final.Logging, want.Logging) {
		t.Errorf("Logging = %+v, want %+v (appconfig.Default disagrees with Run)", final.Logging, want.Logging)
	}
	if len(final.DirectoryScanner.SidecarTypes) != len(want.DirectoryScanner.SidecarTypes) {
		t.Errorf("SidecarTypes count = %d, want %d (appconfig.Default disagrees with Run)",
			len(final.DirectoryScanner.SidecarTypes), len(want.DirectoryScanner.SidecarTypes))
	}
}

func TestRun_OnAFreshInstallOnlySeedsWhatDefaultLeftEmpty(t *testing.T) {
	// mongostore.AppConfigRepo.Get already returns appconfig.Default() when
	// nothing is stored, so a genuinely fresh install's first read is
	// Default(), not appconfig.Config{}. Only API keys and the admin
	// account should still need seeding from there.
	store, backend := newStoreOn(appconfig.Default())

	report, err := Run(context.Background(), store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.APIKeys == nil {
		t.Fatal("expected API keys to be seeded even on a fresh install")
	}
	if report.Admin.Username == "" {
		t.Fatal("expected the admin account to be seeded even on a fresh install")
	}
	if report.SidecarTypesAdded != 0 {
		t.Errorf("SidecarTypesAdded = %d, want 0: the table was already complete", report.SidecarTypesAdded)
	}
	if report.APIKeyIDsBackfilled != 0 {
		t.Errorf("APIKeyIDsBackfilled = %d, want 0: nothing existed yet to backfill", report.APIKeyIDsBackfilled)
	}

	// Two writes: API keys (seeded) and admin (seeded via SeedAdmin).
	// Everything else was already populated by Default() and must not
	// trigger a write.
	if backend.upsertCalls != 2 {
		t.Errorf("upsertCalls = %d, want 2 (api_keys_seed + admin_seed only)", backend.upsertCalls)
	}
}

func TestRun_IsIdempotentOnASecondRun(t *testing.T) {
	store, backend := newStoreOn(&appconfig.Config{})

	if _, err := Run(context.Background(), store); err != nil {
		t.Fatalf("unexpected error on first run: %v", err)
	}
	afterFirst := backend.upsertCalls

	report, err := Run(context.Background(), store)
	if err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}

	if backend.upsertCalls != afterFirst {
		t.Errorf("second run performed %d additional writes, want 0", backend.upsertCalls-afterFirst)
	}
	if report.APIKeys != nil {
		t.Error("second run should not re-report API keys that already existed")
	}
	if report.Admin.Password != "" {
		t.Error("second run should not re-report an admin password that already existed")
	}
}

func TestRun_MergesAndReportsNewlyAddedBuiltinSidecarTypes(t *testing.T) {
	seeded := appconfig.Default()
	// Simulate a database seeded before the last entry in the built-in
	// table existed.
	seeded.DirectoryScanner.SidecarTypes = seeded.DirectoryScanner.SidecarTypes[:len(seeded.DirectoryScanner.SidecarTypes)-1]
	store, _ := newStoreOn(seeded)

	report, err := Run(context.Background(), store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.SidecarTypesAdded != 1 {
		t.Errorf("SidecarTypesAdded = %d, want 1", report.SidecarTypesAdded)
	}
}
