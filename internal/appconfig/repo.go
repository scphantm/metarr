package appconfig

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Repo is the MongoDB-backed store for the singleton Config document.
type Repo struct {
	collection *mongo.Collection
}

// NewRepo opens the singleton config document's collection in database.
func NewRepo(client *mongo.Client, database string) *Repo {
	return &Repo{
		collection: client.Database(database).Collection("app_config"),
	}
}

// Get fetches the singleton config document, returning the default config
// if none has been saved yet.
func (r *Repo) Get(ctx context.Context) (*Config, error) {
	var config Config
	err := r.collection.FindOne(ctx, bson.M{"_id": SingletonID}).Decode(&config)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Default(), nil
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// Upsert replaces the singleton config document with config, creating it if
// it doesn't exist yet. config.ID is always forced to SingletonID so there
// can only ever be one copy of this document in the database.
func (r *Repo) Upsert(ctx context.Context, config *Config) error {
	config.ID = SingletonID
	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": SingletonID}, config, options.Replace().SetUpsert(true))
	return err
}
