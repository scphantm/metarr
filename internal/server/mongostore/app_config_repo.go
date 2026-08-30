package mongostore

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"Metarr/internal/shared/appconfig"
)

// AppConfigRepo is the MongoDB-backed store for the singleton application
// config document.
//
// The document is a generated message (appconfig.Config aliases
// metarr.v1.Config), so it is persisted through appconfig.MarshalStored —
// protojson with proto field names and every field emitted — rather than
// the bson struct tags a hand-written struct would carry. That keeps the
// stored field names snake_case and the document self-describing: it is
// readable directly in the collection with the names an operator already
// recognises, and it lists every setting rather than only the ones that
// differ from zero. The protojson bytes are expanded into a BSON
// subdocument (not stored as an opaque blob) so those properties survive.
type AppConfigRepo struct {
	collection *mongo.Collection
}

// NewAppConfigRepo opens the singleton config document's collection in
// database.
func NewAppConfigRepo(client *mongo.Client, database string) *AppConfigRepo {
	return &AppConfigRepo{
		collection: client.Database(database).Collection("app_config"),
	}
}

// Get fetches the singleton config document, returning the default config
// if none has been saved yet.
func (r *AppConfigRepo) Get(ctx context.Context) (*appconfig.Config, error) {
	var raw bson.M
	err := r.collection.FindOne(ctx, bson.M{"_id": appconfig.SingletonID}).Decode(&raw)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return appconfig.Default(), nil
	}
	if err != nil {
		return nil, err
	}

	// _id is a storage concern, not a setting, so it is not a field on the
	// message — drop it before decoding.
	delete(raw, "_id")

	relaxedJSON, err := bson.MarshalExtJSON(raw, false, false)
	if err != nil {
		return nil, err
	}
	config, err := appconfig.UnmarshalStored(relaxedJSON)
	if err != nil {
		return nil, err
	}
	return appconfig.Normalize(config), nil
}

// Upsert replaces the singleton config document with config, creating it if
// it doesn't exist yet. The document is always stored under SingletonID so
// there can only ever be one copy of it in the database.
func (r *AppConfigRepo) Upsert(ctx context.Context, config *appconfig.Config) error {
	storedJSON, err := appconfig.MarshalStored(config)
	if err != nil {
		return err
	}

	var doc bson.M
	if err := bson.UnmarshalExtJSON(storedJSON, false, &doc); err != nil {
		return err
	}
	doc["_id"] = appconfig.SingletonID

	_, err = r.collection.ReplaceOne(ctx, bson.M{"_id": appconfig.SingletonID}, doc, options.Replace().SetUpsert(true))
	return err
}
