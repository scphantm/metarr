package mongostore

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"Metarr/internal/server/mongostore/versioned"
)

const workflowCollection = "workflows"

// Workflow is a single version of one workflow graph. Every save is a brand
// new document — see versioned.Envelope — so history is append-only.
type Workflow struct {
	versioned.Envelope `bson:",inline"`

	Name        string   `bson:"name"        json:"name"`
	Description string   `bson:"description" json:"description"`
	Tags        []string `bson:"tags"         json:"tags"`

	// SchemaVersion identifies which shape Nodes/Edges are in — see
	// workflow.SchemaVersion. A document without one (the zero value) predates
	// the control/data-edge redesign and is opened read-only in the editor
	// rather than guessed at.
	SchemaVersion int `bson:"schema_version" json:"schema_version"`

	// The canonical graph shape (workflow.Node / workflow.Edge), stored
	// loosely as bson.M rather than the typed Go structs: Mongo doesn't need
	// it typed, and keeping it loose here means a catalog-driven schema
	// change on the frontend never needs a backend release in lockstep — see
	// the no-migration-logic note on reading old-SchemaVersion documents.
	Nodes    []bson.M `bson:"nodes"    json:"nodes"`
	Edges    []bson.M `bson:"edges"    json:"edges"`
	Viewport bson.M   `bson:"viewport" json:"viewport"`
}

func workflowEnvelope(w *Workflow) versioned.Envelope       { return w.Envelope }
func setWorkflowEnvelope(w *Workflow, e versioned.Envelope) { w.Envelope = e }

// WorkflowRepo stores workflow graphs as versioned documents.
type WorkflowRepo struct {
	store *versioned.Store[Workflow]
}

// NewWorkflowRepo opens the workflows collection in database.
func NewWorkflowRepo(client *mongo.Client, database string) *WorkflowRepo {
	return &WorkflowRepo{
		store: versioned.NewStore[Workflow](client, database, workflowCollection, workflowEnvelope, setWorkflowEnvelope),
	}
}

// EnsureIndexes creates the indexes workflow reads depend on.
func (r *WorkflowRepo) EnsureIndexes(ctx context.Context) error {
	return r.store.EnsureIndexes(ctx)
}

// Save creates version 1 of a new workflow if documentID is the zero value,
// or appends a new version to an existing one otherwise.
func (r *WorkflowRepo) Save(ctx context.Context, documentID bson.ObjectID, w Workflow) (Workflow, error) {
	return r.store.SaveNewVersion(ctx, documentID, w)
}

// ListLatest returns the latest version of every workflow, newest first.
func (r *WorkflowRepo) ListLatest(ctx context.Context, filter versioned.LatestFilter) ([]Workflow, string, bool, error) {
	return r.store.ListLatest(ctx, filter)
}

// GetLatest fetches the newest version of one workflow.
func (r *WorkflowRepo) GetLatest(ctx context.Context, documentID bson.ObjectID) (Workflow, error) {
	return r.store.GetLatest(ctx, documentID)
}

// GetVersion fetches one specific version of one workflow.
func (r *WorkflowRepo) GetVersion(ctx context.Context, documentID bson.ObjectID, version int) (Workflow, error) {
	return r.store.GetVersion(ctx, documentID, version)
}

// ListVersions returns every version of one workflow, newest first.
func (r *WorkflowRepo) ListVersions(ctx context.Context, documentID bson.ObjectID) ([]Workflow, error) {
	return r.store.ListVersions(ctx, documentID)
}
