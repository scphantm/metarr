package mongostore

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"Metarr/internal/server/mongostore/versioned"
)

// connectTestWorkflowRepo opens a WorkflowRepo against a real MongoDB
// instance. Skips cleanly (rather than failing) when no MongoDB is
// reachable, so `go test ./...` still passes without one running.
//
// The metarr app user is scoped to the metarr database only (authSource
// metarr), and WorkflowRepo hardcodes the "workflows" collection name, so
// this runs against the real dev database rather than an isolated one — the
// returned cleanup function deletes only the specific document(s) a test
// created, by id, so it never disturbs an unrelated workflow saved there.
func connectTestWorkflowRepo(t *testing.T) (repo *WorkflowRepo, cleanupDocument func(bson.ObjectID)) {
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

	repo = NewWorkflowRepo(client, "metarr")
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes() error = %v", err)
	}

	t.Cleanup(func() {
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		_ = client.Disconnect(disconnectCtx)
	})

	cleanupDocument = func(documentID bson.ObjectID) {
		t.Cleanup(func() {
			deleteCtx, deleteCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer deleteCancel()
			_, _ = client.Database("metarr").Collection(workflowCollection).
				DeleteMany(deleteCtx, bson.M{"document_id": documentID})
		})
	}
	return repo, cleanupDocument
}

func TestWorkflowRepoRoundTrip(t *testing.T) {
	repo, cleanupDocument := connectTestWorkflowRepo(t)
	ctx := context.Background()

	draft := Workflow{
		Name:        "Import new episodes",
		Description: "Watches a folder and imports anything new",
		Tags:        []string{"tv", "import"},
		Nodes:       []bson.M{{"id": "n1", "type": "catalogNode"}},
		Edges:       []bson.M{},
		Viewport:    bson.M{"x": 0, "y": 0, "zoom": 1},
	}

	v1, err := repo.Save(ctx, bson.NilObjectID, draft)
	if err != nil {
		t.Fatalf("Save(v1) error = %v", err)
	}
	cleanupDocument(v1.DocumentID)
	if v1.Version != 1 {
		t.Errorf("Version = %d, want 1", v1.Version)
	}

	draft.DocumentID = v1.DocumentID
	draft.Tags = append(draft.Tags, "v2-tag")
	v2, err := repo.Save(ctx, v1.DocumentID, draft)
	if err != nil {
		t.Fatalf("Save(v2) error = %v", err)
	}
	if v2.Version != 2 {
		t.Errorf("Version = %d, want 2", v2.Version)
	}

	latest, err := repo.GetLatest(ctx, v1.DocumentID)
	if err != nil {
		t.Fatalf("GetLatest() error = %v", err)
	}
	if len(latest.Tags) != 3 {
		t.Errorf("GetLatest().Tags = %v, want the v2 tag set", latest.Tags)
	}

	versions, err := repo.ListVersions(ctx, v1.DocumentID)
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2", len(versions))
	}

	// ListLatest runs against the shared dev collection, which may already
	// hold unrelated workflows, so this only confirms this test's own
	// document surfaces at its latest version — not the total row count.
	list, _, _, err := repo.ListLatest(ctx, versioned.LatestFilter{Limit: 100})
	if err != nil {
		t.Fatalf("ListLatest() error = %v", err)
	}
	found := false
	for _, w := range list {
		if w.DocumentID == v1.DocumentID {
			found = true
			if w.Version != 2 {
				t.Errorf("ListLatest found this workflow at version %d, want 2", w.Version)
			}
		}
	}
	if !found {
		t.Error("ListLatest did not include this test's workflow")
	}
}
