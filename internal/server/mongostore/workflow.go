package mongostore

import (
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

// NewWorkflowStore opens the workflows collection as a versioned store of
// workflow graphs. It returns the concrete *versioned.Store directly — the
// Mongo-vs-fake boundary WorkflowServer tests against is versioned.Store
// itself, not a wrapper around it — so this is only the type-specific
// plumbing (collection name, envelope accessors) the generic store needs,
// not a persistence seam of its own.
func NewWorkflowStore(client *mongo.Client, database string) *versioned.Store[Workflow] {
	return versioned.NewStore[Workflow](client, database, workflowCollection, workflowEnvelope, setWorkflowEnvelope)
}
