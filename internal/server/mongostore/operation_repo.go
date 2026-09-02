package mongostore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/encoding/protojson"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

const operationCollection = "config_operations"

// operationTTL is how long a completed (or abandoned) operation record lives
// before Mongo's TTL monitor reaps it. AIP-151 lets a server drop finished
// operations; the config UI polls one to completion within seconds, so a day
// is generous headroom for a slow client or a paused tab.
const operationTTL = 24 * time.Hour

// OperationRepo is the MongoDB-backed store for AIP-151 long-running operations
// behind the config API. It is server-only — agents never see it — and every
// record is keyed by its resource name (`operations/{correlation_id}`), the
// same correlation id the originating write stamps on its system_config_update
// event.
//
// The stored document is the generated metarrv1.Operation marshalled with
// protojson (the AppConfigRepo technique: snake_case field names, readable in
// the collection), plus a `created_at` BSON date the TTL index runs off. Both
// Create and Complete upsert on `_id`, so the RPC recording the operation and
// the listener finishing it can arrive in either order without losing the
// record or overwriting the other's fields.
type OperationRepo struct {
	collection *mongo.Collection
}

// NewOperationRepo opens the config-operations collection in database.
func NewOperationRepo(client *mongo.Client, database string) *OperationRepo {
	return &OperationRepo{
		collection: client.Database(database).Collection(operationCollection),
	}
}

// EnsureIndexes creates the TTL index that reaps finished operation records.
// Idempotent, so it is safe on every startup.
func (r *OperationRepo) EnsureIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "created_at", Value: 1}},
		Options: options.Index().
			SetName("created_at_ttl").
			SetExpireAfterSeconds(int32(operationTTL.Seconds())),
	})
	if err != nil {
		return fmt.Errorf("mongostore: creating %s indexes: %w", operationCollection, err)
	}
	return nil
}

// Create records a new, unfinished operation. If the listener has already
// completed one under this name (the finish raced ahead of the RPC return),
// the existing record is left untouched.
func (r *OperationRepo) Create(ctx context.Context, name string) error {
	_, err := r.collection.UpdateOne(
		ctx,
		bson.M{"_id": name},
		bson.M{
			"$setOnInsert": bson.M{
				"name":       name,
				"done":       false,
				"created_at": bson.NewDateTimeFromTime(time.Now()),
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("mongostore: recording operation %q: %w", name, err)
	}
	return nil
}

// Complete marks the operation done. A zero opCode is success; otherwise
// `error` is set to {opCode, opMessage} (opCode is a connect.Code / google.rpc
// Code integer). The record is upserted, so a listener that runs before the
// RPC has recorded the operation still produces a complete, reapable document.
func (r *OperationRepo) Complete(ctx context.Context, name string, opCode int32, opMessage string) error {
	set := bson.M{"name": name, "done": true}
	unset := bson.M{}
	if opCode != 0 || opMessage != "" {
		set["error"] = bson.M{"code": opCode, "message": opMessage}
		unset["response"] = ""
	} else {
		unset["error"] = ""
	}

	update := bson.M{
		"$set":         set,
		"$setOnInsert": bson.M{"created_at": bson.NewDateTimeFromTime(time.Now())},
	}
	if len(unset) > 0 {
		update["$unset"] = unset
	}

	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": name}, update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("mongostore: completing operation %q: %w", name, err)
	}
	return nil
}

// Get returns the operation, or (nil, nil) when no record exists — the caller
// maps that to NotFound.
func (r *OperationRepo) Get(ctx context.Context, name string) (*metarrv1.Operation, error) {
	var raw bson.M
	err := r.collection.FindOne(ctx, bson.M{"_id": name}).Decode(&raw)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return docToOperation(raw)
}

// List returns operations newest-first. done, when non-nil, filters on
// completion state; limit caps the result (0 means no cap). Pagination is
// offset-free — the collection is tiny and TTL-reaped — so there is no cursor.
func (r *OperationRepo) List(ctx context.Context, done *bool, limit int64) ([]*metarrv1.Operation, error) {
	filter := bson.M{}
	if done != nil {
		filter["done"] = *done
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(limit)
	}

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var out []*metarrv1.Operation
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, err
		}
		op, err := docToOperation(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return out, cursor.Err()
}

// docToOperation rebuilds the generated message from a stored document. The
// storage-only keys (`_id`, `created_at`) are dropped first; the rest is the
// protojson form the message marshals to.
func docToOperation(raw bson.M) (*metarrv1.Operation, error) {
	delete(raw, "_id")
	delete(raw, "created_at")

	relaxedJSON, err := bson.MarshalExtJSON(raw, false, false)
	if err != nil {
		return nil, err
	}
	var op metarrv1.Operation
	if err := protojson.Unmarshal(relaxedJSON, &op); err != nil {
		return nil, err
	}
	return &op, nil
}
