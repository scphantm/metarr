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
	var config appconfig.Config
	err := r.collection.FindOne(ctx, bson.M{"_id": appconfig.SingletonID}).Decode(&config)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return appconfig.Default(), nil
	}
	if err != nil {
		return nil, err
	}
	return appconfig.Normalize(&config), nil
}

// Upsert replaces the singleton config document with config, creating it if
// it doesn't exist yet. config.ID is always forced to SingletonID so there
// can only ever be one copy of this document in the database.
func (r *AppConfigRepo) Upsert(ctx context.Context, config *appconfig.Config) error {
	config.ID = appconfig.SingletonID
	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": appconfig.SingletonID}, config, options.Replace().SetUpsert(true))
	return err
}
