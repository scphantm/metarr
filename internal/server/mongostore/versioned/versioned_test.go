package versioned

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// testDoc is the smallest possible consumer of Store[T]: an embedded
// Envelope plus one payload field.
type testDoc struct {
	Envelope `bson:",inline"`
	Value    string `bson:"value" json:"value"`
}

func testDocEnvelope(d *testDoc) Envelope       { return d.Envelope }
func setTestDocEnvelope(d *testDoc, e Envelope) { d.Envelope = e }

// connectTestStore opens a Store[testDoc] against a real MongoDB instance in
// its own collection, dropped on test cleanup. This package has no mock for
// the Mongo driver, so SaveNewVersion's flip-then-insert atomicity and
// ListLatest's cursor pagination can only be verified end to end; the test
// skips cleanly (rather than failing) when no MongoDB is reachable, so
// `go test ./...` still passes in an environment without one running.
func connectTestStore(t *testing.T) *Store[testDoc] {
	t.Helper()

	uri := os.Getenv("METARR_TEST_MONGO_URI")
	if uri == "" {
		uri = "mongodb://metarr:metarr@localhost:27017/metarr?authSource=metarr"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Skipf("mongo.Connect: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("no reachable MongoDB at %s: %v", uri, err)
	}

	// The metarr app user is scoped to the metarr database only (authSource
	// metarr), so tests run there too rather than an arbitrary test database
	// — in a collection unique to this test, dropped on cleanup, never the
	// database itself.
	database := "metarr"
	collection := "test_versioned_" + t.Name()
	store := NewStore[testDoc](client, database, collection, testDocEnvelope, setTestDocEnvelope)

	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer dropCancel()
		_ = client.Database(database).Collection(collection).Drop(dropCtx)
		_ = client.Disconnect(dropCtx)
	})

	return store
}

func TestSaveNewVersionStartsAtOne(t *testing.T) {
	store := connectTestStore(t)
	ctx := context.Background()

	saved, err := store.SaveNewVersion(ctx, bson.NilObjectID, testDoc{Value: "v1"})
	if err != nil {
		t.Fatalf("SaveNewVersion() error = %v", err)
	}
	if saved.Version != 1 {
		t.Errorf("Version = %d, want 1", saved.Version)
	}
	if !saved.IsLatest {
		t.Error("IsLatest = false, want true for a freshly saved document")
	}
	if saved.DocumentID == bson.NilObjectID {
		t.Error("DocumentID was not minted")
	}
}

func TestSaveNewVersionAppendsAndFlipsLatest(t *testing.T) {
	store := connectTestStore(t)
	ctx := context.Background()

	v1, err := store.SaveNewVersion(ctx, bson.NilObjectID, testDoc{Value: "v1"})
	if err != nil {
		t.Fatalf("SaveNewVersion(v1) error = %v", err)
	}

	v2, err := store.SaveNewVersion(ctx, v1.DocumentID, testDoc{Value: "v2"})
	if err != nil {
		t.Fatalf("SaveNewVersion(v2) error = %v", err)
	}
	if v2.Version != 2 {
		t.Errorf("Version = %d, want 2", v2.Version)
	}
	if v2.DocumentID != v1.DocumentID {
		t.Errorf("DocumentID changed across versions: %s -> %s", v1.DocumentID.Hex(), v2.DocumentID.Hex())
	}

	// v1 must no longer be latest, and GetLatest must return v2.
	stale, err := store.GetVersion(ctx, v1.DocumentID, 1)
	if err != nil {
		t.Fatalf("GetVersion(1) error = %v", err)
	}
	if stale.IsLatest {
		t.Error("version 1 is still marked latest after version 2 was saved")
	}

	latest, err := store.GetLatest(ctx, v1.DocumentID)
	if err != nil {
		t.Fatalf("GetLatest() error = %v", err)
	}
	if latest.Value != "v2" || latest.Version != 2 {
		t.Errorf("GetLatest() = %+v, want version 2 / value v2", latest)
	}
}

func TestGetVersionIsUnaffectedByLaterSaves(t *testing.T) {
	store := connectTestStore(t)
	ctx := context.Background()

	v1, err := store.SaveNewVersion(ctx, bson.NilObjectID, testDoc{Value: "original"})
	if err != nil {
		t.Fatalf("SaveNewVersion(v1) error = %v", err)
	}
	if _, err := store.SaveNewVersion(ctx, v1.DocumentID, testDoc{Value: "changed"}); err != nil {
		t.Fatalf("SaveNewVersion(v2) error = %v", err)
	}

	original, err := store.GetVersion(ctx, v1.DocumentID, 1)
	if err != nil {
		t.Fatalf("GetVersion(1) error = %v", err)
	}
	if original.Value != "original" {
		t.Errorf("Value = %q, want %q (version 1 must read back exactly as saved)", original.Value, "original")
	}
}

func TestListLatestReturnsOnlyNewestPerDocument(t *testing.T) {
	store := connectTestStore(t)
	ctx := context.Background()

	first, err := store.SaveNewVersion(ctx, bson.NilObjectID, testDoc{Value: "a-v1"})
	if err != nil {
		t.Fatalf("SaveNewVersion() error = %v", err)
	}
	if _, err := store.SaveNewVersion(ctx, first.DocumentID, testDoc{Value: "a-v2"}); err != nil {
		t.Fatalf("SaveNewVersion() error = %v", err)
	}
	if _, err := store.SaveNewVersion(ctx, bson.NilObjectID, testDoc{Value: "b-v1"}); err != nil {
		t.Fatalf("SaveNewVersion() error = %v", err)
	}

	items, _, hasMore, err := store.ListLatest(ctx, LatestFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListLatest() error = %v", err)
	}
	if hasMore {
		t.Error("hasMore = true, want false with only 2 documents under a limit of 10")
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2 (one per document, latest only)", len(items))
	}
	for _, item := range items {
		if item.Value == "a-v1" {
			t.Errorf("ListLatest returned a stale version: %+v", item)
		}
	}
}

func TestListLatestPaginatesByCursor(t *testing.T) {
	store := connectTestStore(t)
	ctx := context.Background()

	const total = 5
	for i := 0; i < total; i++ {
		if _, err := store.SaveNewVersion(ctx, bson.NilObjectID, testDoc{Value: "doc"}); err != nil {
			t.Fatalf("SaveNewVersion() error = %v", err)
		}
	}

	seen := map[bson.ObjectID]bool{}
	cursor := bson.NilObjectID
	pages := 0
	for {
		items, nextCursor, hasMore, err := store.ListLatest(ctx, LatestFilter{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatalf("ListLatest() error = %v", err)
		}
		pages++
		for _, item := range items {
			if seen[item.ID] {
				t.Errorf("duplicate item across pages: %s", item.ID.Hex())
			}
			seen[item.ID] = true
		}
		if !hasMore {
			break
		}
		if nextCursor == "" {
			t.Fatal("hasMore = true but next_cursor is empty")
		}
		parsed, err := bson.ObjectIDFromHex(nextCursor)
		if err != nil {
			t.Fatalf("next_cursor %q is not a valid ObjectID: %v", nextCursor, err)
		}
		cursor = parsed
		if pages > total {
			t.Fatal("pagination did not terminate")
		}
	}

	if len(seen) != total {
		t.Errorf("saw %d distinct items across %d pages, want %d", len(seen), pages, total)
	}
}

func TestGetLatestNotFound(t *testing.T) {
	store := connectTestStore(t)
	ctx := context.Background()

	_, err := store.GetLatest(ctx, bson.NewObjectID())
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetLatest() error = %v, want ErrNotFound", err)
	}
}
