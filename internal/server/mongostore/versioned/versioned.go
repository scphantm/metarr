// Package versioned implements a generic "versioned Mongo document" pattern:
// every save inserts a brand new document rather than mutating one in place,
// so full history is preserved and independently readable. One field,
// is_latest, is flipped on every save so "list only the newest version of
// each document" stays a cheap indexed query instead of a scan-and-compare.
//
// This package knows nothing about any specific document type — Workflow
// (internal/server/mongostore/workflow_repo.go) is its first consumer, but it
// is meant to be reused by future versioned document types too.
package versioned

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ErrNotFound is returned when no version matches the request. It is a
// separate sentinel from mongostore.ErrNotFound because this package sits
// below mongostore in the dependency graph and cannot import it back.
var ErrNotFound = errors.New("versioned: no matching record")

// Envelope is the bookkeeping every versioned document carries. Embed it
// (bson:",inline") into a concrete document type:
//
//	type Workflow struct {
//	    versioned.Envelope `bson:",inline"`
//	    Name string `bson:"name" json:"name"`
//	}
//
// DocumentID groups every version of the same logical document together and
// is minted once, when version 1 is created. ID is this specific version's
// own identity — every version is a fully independent Mongo document.
type Envelope struct {
	ID         bson.ObjectID `bson:"_id,omitempty" json:"id"`
	DocumentID bson.ObjectID `bson:"document_id"    json:"document_id"`
	Version    int           `bson:"version"        json:"version"`
	// IsLatest is a storage-layer detail, not part of the API — latest-ness
	// is implied by which endpoint was called, not a field callers inspect.
	IsLatest  bool      `bson:"is_latest" json:"-"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

// LatestFilter narrows a ListLatest call. Cursor is the _id of the last item
// from the previous page; the zero value requests the first page.
type LatestFilter struct {
	Limit  int64
	Cursor bson.ObjectID
}

const defaultListLimit = 20

// Store wraps one Mongo collection holding versioned documents of type T. The
// getEnvelope/setEnvelope closures let Store read and stamp the bookkeeping
// fields without reflection or an interface method that would collide with
// T's embedded Envelope field name.
type Store[T any] struct {
	collection  *mongo.Collection
	getEnvelope func(*T) Envelope
	setEnvelope func(*T, Envelope)
}

// NewStore opens collectionName in database for versioned documents of type T.
func NewStore[T any](
	client *mongo.Client,
	database, collectionName string,
	getEnvelope func(*T) Envelope,
	setEnvelope func(*T, Envelope),
) *Store[T] {
	return &Store[T]{
		collection:  client.Database(database).Collection(collectionName),
		getEnvelope: getEnvelope,
		setEnvelope: setEnvelope,
	}
}

// EnsureIndexes creates the indexes every versioned collection depends on.
// Creation is idempotent, so this is safe to call on every startup.
func (s *Store[T]) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			// Defends the one-version-per-number invariant even though the
			// only writer (SaveNewVersion) already maintains it.
			Keys:    bson.D{{Key: "document_id", Value: 1}, {Key: "version", Value: 1}},
			Options: options.Index().SetName("document_id_version_unique").SetUnique(true),
		},
		{
			// Drives both GetLatest and the ListLatest listing.
			Keys:    bson.D{{Key: "document_id", Value: 1}, {Key: "is_latest", Value: 1}},
			Options: options.Index().SetName("document_id_is_latest"),
		},
	}

	if _, err := s.collection.Indexes().CreateMany(ctx, indexes); err != nil {
		return fmt.Errorf("versioned: creating indexes: %w", err)
	}
	return nil
}

// SaveNewVersion inserts a new version of the document identified by
// documentID, or starts a brand new document if documentID is the zero
// value. It never mutates a previous version in place.
//
// Atomicity: this is two writes — unmark the previous latest, then insert the
// new one — not one transaction. This package has no session/transaction
// precedent elsewhere in the codebase, and a standalone (non-replica-set) dev
// Mongo instance may not support them. If the insert fails after the unmark
// succeeds, the document is briefly "latest-less"; that's recoverable (every
// version is still found by ListVersions, and a retried save fixes it) and
// simpler than adding transaction machinery for a single-collection flip.
func (s *Store[T]) SaveNewVersion(ctx context.Context, documentID bson.ObjectID, doc T) (T, error) {
	var zero T

	nextVersion := 1
	if documentID != bson.NilObjectID {
		current, err := s.GetLatest(ctx, documentID)
		switch {
		case err == nil:
			nextVersion = s.getEnvelope(&current).Version + 1
		case errors.Is(err, ErrNotFound):
			// documentID was supplied but nothing exists yet under it —
			// treat as the first version, same as the zero-value case.
		default:
			return zero, fmt.Errorf("versioned: reading current version: %w", err)
		}
	} else {
		documentID = bson.NewObjectID()
	}

	if _, err := s.collection.UpdateMany(ctx,
		bson.M{"document_id": documentID, "is_latest": true},
		bson.M{"$set": bson.M{"is_latest": false}},
	); err != nil {
		return zero, fmt.Errorf("versioned: unmarking previous latest version: %w", err)
	}

	s.setEnvelope(&doc, Envelope{
		ID:         bson.NewObjectID(),
		DocumentID: documentID,
		Version:    nextVersion,
		IsLatest:   true,
		CreatedAt:  time.Now().UTC(),
	})

	if _, err := s.collection.InsertOne(ctx, doc); err != nil {
		return zero, fmt.Errorf("versioned: inserting version %d: %w", nextVersion, err)
	}
	return doc, nil
}

// ListLatest returns the latest version of every document, newest first,
// paginated by an opaque cursor (the hex _id of the last row returned).
func (s *Store[T]) ListLatest(ctx context.Context, filter LatestFilter) ([]T, string, bool, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}

	query := bson.M{"is_latest": true}
	if filter.Cursor != bson.NilObjectID {
		query["_id"] = bson.M{"$lt": filter.Cursor}
	}

	// Fetch one extra row to learn hasMore without a separate count query.
	findOptions := options.Find().
		SetSort(bson.D{{Key: "_id", Value: -1}}).
		SetLimit(limit + 1)

	cursor, err := s.collection.Find(ctx, query, findOptions)
	if err != nil {
		return nil, "", false, fmt.Errorf("versioned: listing latest versions: %w", err)
	}

	items := []T{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, "", false, fmt.Errorf("versioned: decoding latest versions: %w", err)
	}

	hasMore := int64(len(items)) > limit
	if hasMore {
		items = items[:limit]
	}

	var nextCursor string
	if hasMore && len(items) > 0 {
		nextCursor = s.getEnvelope(&items[len(items)-1]).ID.Hex()
	}

	return items, nextCursor, hasMore, nil
}

// GetLatest fetches the newest version of documentID.
func (s *Store[T]) GetLatest(ctx context.Context, documentID bson.ObjectID) (T, error) {
	var doc T
	err := s.collection.FindOne(ctx, bson.M{"document_id": documentID, "is_latest": true}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return doc, ErrNotFound
	}
	if err != nil {
		return doc, fmt.Errorf("versioned: fetching latest version of %s: %w", documentID.Hex(), err)
	}
	return doc, nil
}

// GetVersion fetches one specific version of documentID.
func (s *Store[T]) GetVersion(ctx context.Context, documentID bson.ObjectID, version int) (T, error) {
	var doc T
	err := s.collection.FindOne(ctx, bson.M{"document_id": documentID, "version": version}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return doc, ErrNotFound
	}
	if err != nil {
		return doc, fmt.Errorf("versioned: fetching version %d of %s: %w", version, documentID.Hex(), err)
	}
	return doc, nil
}

// ListVersions returns every version of documentID, newest first.
func (s *Store[T]) ListVersions(ctx context.Context, documentID bson.ObjectID) ([]T, error) {
	findOptions := options.Find().SetSort(bson.D{{Key: "version", Value: -1}})

	cursor, err := s.collection.Find(ctx, bson.M{"document_id": documentID}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("versioned: listing versions of %s: %w", documentID.Hex(), err)
	}

	items := []T{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, fmt.Errorf("versioned: decoding versions of %s: %w", documentID.Hex(), err)
	}
	return items, nil
}
