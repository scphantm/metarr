package mongostore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"Metarr/internal/shared/scanmodel"
)

// localDirectoryCollection holds both record kinds a scan produces: one
// document per media item directory, and one per media file linked back to its
// directory. They are told apart by the record_type field.
const localDirectoryCollection = "local_directory"

// ErrNotFound is returned when no record matches the requested id.
var ErrNotFound = errors.New("mongostore: no matching record")

// LocalDirectoryRepo stores the results of directory scans. The collection is
// treated as a rebuildable cache of what is on disk: a rescan replaces what it
// finds and sweeps away what it no longer does.
type LocalDirectoryRepo struct {
	collection *mongo.Collection
}

// NewLocalDirectoryRepo opens the local_directory collection in database.
func NewLocalDirectoryRepo(client *mongo.Client, database string) *LocalDirectoryRepo {
	return &LocalDirectoryRepo{
		collection: client.Database(database).Collection(localDirectoryCollection),
	}
}

// EnsureIndexes creates the indexes the scanner and its readers depend on.
// Creation is idempotent, so this is safe to call on every startup; there is no
// migration system to hang it off instead.
//
// The unique index on path is load-bearing rather than an optimization: path is
// the natural key every upsert matches on, and without the uniqueness
// constraint a concurrent rescan could insert a second document for the same
// directory.
func (r *LocalDirectoryRepo) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "path", Value: 1}},
			Options: options.Index().SetName("path_unique").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "record_type", Value: 1}},
			Options: options.Index().SetName("record_type"),
		},
		{
			// Fetches a directory's media files.
			Keys:    bson.D{{Key: "directory_id", Value: 1}},
			Options: options.Index().SetName("directory_id"),
		},
		{
			// Drives the stale sweep at the end of a scan.
			Keys:    bson.D{{Key: "scan_root_path", Value: 1}, {Key: "scanned_at", Value: 1}},
			Options: options.Index().SetName("scan_root_scanned_at"),
		},
		{
			Keys:    bson.D{{Key: "type", Value: 1}},
			Options: options.Index().SetName("type"),
		},
		{
			// Provider-id lookups, which is how reconciliation against Sonarr
			// and the metadata databases will find a local item. Both record
			// kinds keep their ids on their own metadata record — the series'
			// own on the directory, an episode's on its media file — so one
			// index serves FindByExternalLink and FindByEpisodeID alike.
			Keys:    bson.D{{Key: "metadata.external_links.key", Value: 1}, {Key: "metadata.external_links.value", Value: 1}},
			Options: options.Index().SetName("metadata_external_links"),
		},
	}

	if _, err := r.collection.Indexes().CreateMany(ctx, indexes); err != nil {
		return fmt.Errorf("mongostore: creating %s indexes: %w", localDirectoryCollection, err)
	}
	return nil
}

// ReplaceScanResults persists everything one library scan produced and removes
// the records it no longer found.
//
// The ordering is forced by how ids are assigned. MongoDB mints _id on insert,
// so a media file's directory_id does not exist until its directory has been
// written — hence the directory is upserted first and its id read back before
// its media files are queued.
//
// Records are written as whole-document replacements keyed on path, so a field
// that is empty this time round genuinely becomes empty rather than keeping its
// old value; see replacementDocumentFrom. Because the marshaled document carries
// no _id, MongoDB retains the existing one when replacing and assigns a fresh
// one when inserting, which keeps ids stable across rescans and leaves
// directory_id links and any external reference to a record valid.
func (r *LocalDirectoryRepo) ReplaceScanResults(
	ctx context.Context,
	scanRootPath string,
	results []*scanmodel.ScanResult,
	scanStartedAt time.Time,
) error {
	for _, result := range results {
		if err := r.UpsertScanResult(ctx, scanRootPath, result); err != nil {
			return err
		}
	}
	return r.DeleteStaleRecords(ctx, scanRootPath, scanStartedAt)
}

// UpsertScanResult persists one scanned item directory and its media files.
//
// Scans arrive from an agent one item at a time rather than as a single payload
// at the end, so this is the unit the ingestion path works in: the library fills
// in progressively, and a scan of a large library never has to be held whole in
// memory on either side. The stale sweep is a separate step, run once when the
// agent reports the scan complete — see DeleteStaleRecords.
//
// A result carrying media files but no directory is a later part of an item too
// large to send in one message. Its files are written against the directory
// record part zero already created, which is why the directory id is looked up
// rather than assumed.
func (r *LocalDirectoryRepo) UpsertScanResult(
	ctx context.Context,
	scanRootPath string,
	result *scanmodel.ScanResult,
) error {
	if result == nil {
		return nil
	}

	var (
		directoryID bson.ObjectID
		err         error
	)

	if result.Directory != nil {
		result.Directory.ScanRootPath = scanRootPath
		directoryID, err = r.upsertDirectory(ctx, result.Directory)
		if err != nil {
			return err
		}
	} else if len(result.MediaFiles) > 0 {
		directoryID, err = r.directoryIDForPath(ctx, result.MediaFiles[0].DirectoryPath)
		if err != nil {
			return err
		}
	}

	if len(result.MediaFiles) == 0 {
		return nil
	}

	mediaFileWrites := make([]mongo.WriteModel, 0, len(result.MediaFiles))
	for i := range result.MediaFiles {
		mediaFile := result.MediaFiles[i]
		mediaFile.DirectoryID = directoryID
		mediaFile.ScanRootPath = scanRootPath

		replacement, err := replacementDocumentFrom(mediaFile)
		if err != nil {
			return fmt.Errorf("mongostore: encoding media file %s: %w", mediaFile.Path, err)
		}
		mediaFileWrites = append(mediaFileWrites,
			mongo.NewReplaceOneModel().
				SetFilter(bson.M{"path": mediaFile.Path}).
				SetReplacement(replacement).
				SetUpsert(true),
		)
	}

	if _, err := r.collection.BulkWrite(ctx, mediaFileWrites, options.BulkWrite().SetOrdered(false)); err != nil {
		return fmt.Errorf("mongostore: writing media file records: %w", err)
	}
	return nil
}

// directoryIDForPath finds an already-written directory record's id. A missing
// one is not an error: the media files are still stored, just unlinked, which
// is recoverable on the next scan and better than dropping them.
func (r *LocalDirectoryRepo) directoryIDForPath(ctx context.Context, directoryPath string) (bson.ObjectID, error) {
	if directoryPath == "" {
		return bson.NilObjectID, nil
	}

	var stored struct {
		ID bson.ObjectID `bson:"_id"`
	}
	err := r.collection.FindOne(
		ctx,
		bson.M{"path": directoryPath, "record_type": scanmodel.RecordTypeTVSeries},
		options.FindOne().SetProjection(bson.M{"_id": 1}),
	).Decode(&stored)

	if errors.Is(err, mongo.ErrNoDocuments) {
		return bson.NilObjectID, nil
	}
	if err != nil {
		return bson.NilObjectID, fmt.Errorf("mongostore: finding directory %s: %w", directoryPath, err)
	}
	return stored.ID, nil
}

// upsertDirectory writes one directory record and returns its id, creating the
// document if this is the first time the directory has been seen.
//
// FindOneAndReplace is used rather than a plain replace because it reports the
// _id in the same round trip whether the document was just inserted or already
// existed, which the media file records need before they can be written.
func (r *LocalDirectoryRepo) upsertDirectory(ctx context.Context, directory *scanmodel.TVSeries) (bson.ObjectID, error) {
	replacement, err := replacementDocumentFrom(*directory)
	if err != nil {
		return bson.NilObjectID, fmt.Errorf("mongostore: encoding directory %s: %w", directory.Path, err)
	}

	findOptions := options.FindOneAndReplace().
		SetUpsert(true).
		SetReturnDocument(options.After).
		SetProjection(bson.M{"_id": 1})

	var stored struct {
		ID bson.ObjectID `bson:"_id"`
	}
	err = r.collection.FindOneAndReplace(ctx, bson.M{"path": directory.Path}, replacement, findOptions).Decode(&stored)
	if err != nil {
		return bson.NilObjectID, fmt.Errorf("mongostore: upserting directory %s: %w", directory.Path, err)
	}

	directory.ID = stored.ID
	return stored.ID, nil
}

// DeleteStaleRecords removes everything under this scan root that the current
// scan did not touch, which is how directories and files deleted from disk leave
// the cache. It sweeps both record kinds, so a removed directory takes its media
// file records with it rather than orphaning them.
//
// Deleting after the writes, rather than clearing the scan root first, means
// there is never a window where the library reads as empty. scanStartedAt must
// be the timestamp taken before the walk began: anything older than that under
// this root is something the scan looked for and did not find.
//
// This runs once, when a scan is reported complete. A scan that fails part way
// through must never reach it — sweeping on a partial scan would delete the
// half the agent never got to.
func (r *LocalDirectoryRepo) DeleteStaleRecords(ctx context.Context, scanRootPath string, scanStartedAt time.Time) error {
	filter := bson.M{
		"scan_root_path": scanRootPath,
		"scanned_at":     bson.M{"$lt": scanStartedAt},
	}
	if _, err := r.collection.DeleteMany(ctx, filter); err != nil {
		return fmt.Errorf("mongostore: removing stale records under %s: %w", scanRootPath, err)
	}
	return nil
}

// replacementDocumentFrom encodes record as a whole-document replacement,
// dropping _id.
//
// A replacement rather than a $set update is essential here, not a style
// preference. Most fields on these records are tagged omitempty, so a $set built
// from a rescan simply would not mention a field that had become empty — and
// MongoDB would leave the previous value in place. Warnings that had been fixed,
// metadata from a deleted NFO, subtitles that are no longer on disk: all of it
// would survive forever, which is the opposite of the rebuildable-cache
// behaviour this collection is meant to have. Replacing the document makes
// absent fields actually absent.
//
// Dropping _id is what preserves identity: with the field omitted, MongoDB keeps
// the existing _id when replacing and mints a new one when inserting.
func replacementDocumentFrom(record any) (bson.M, error) {
	raw, err := bson.Marshal(record)
	if err != nil {
		return nil, err
	}

	var fields bson.M
	if err := bson.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	delete(fields, "_id")

	return fields, nil
}

// ListFilter narrows a directory listing.
type ListFilter struct {
	ScanRootPath string
	Type         scanmodel.DirectoryType
	Limit        int64
	Skip         int64
}

// ListDirectories returns directory records matching filter, newest scan first.
func (r *LocalDirectoryRepo) ListDirectories(ctx context.Context, filter ListFilter) ([]scanmodel.TVSeries, error) {
	query := bson.M{"record_type": scanmodel.RecordTypeTVSeries}
	if filter.ScanRootPath != "" {
		query["scan_root_path"] = filter.ScanRootPath
	}
	if filter.Type != "" {
		query["type"] = filter.Type
	}

	findOptions := options.Find().SetSort(bson.D{{Key: "path", Value: 1}})
	if filter.Limit > 0 {
		findOptions.SetLimit(filter.Limit)
	}
	if filter.Skip > 0 {
		findOptions.SetSkip(filter.Skip)
	}

	cursor, err := r.collection.Find(ctx, query, findOptions)
	if err != nil {
		return nil, fmt.Errorf("mongostore: listing directories: %w", err)
	}

	directories := []scanmodel.TVSeries{}
	if err := cursor.All(ctx, &directories); err != nil {
		return nil, fmt.Errorf("mongostore: decoding directories: %w", err)
	}
	return directories, nil
}

// GetDirectory fetches one directory record by id.
func (r *LocalDirectoryRepo) GetDirectory(ctx context.Context, id bson.ObjectID) (*scanmodel.TVSeries, error) {
	query := bson.M{"_id": id, "record_type": scanmodel.RecordTypeTVSeries}

	var directory scanmodel.TVSeries
	err := r.collection.FindOne(ctx, query).Decode(&directory)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mongostore: fetching directory %s: %w", id.Hex(), err)
	}
	return &directory, nil
}

// ListMediaFiles returns the media file records belonging to one directory.
func (r *LocalDirectoryRepo) ListMediaFiles(ctx context.Context, directoryID bson.ObjectID) ([]scanmodel.MediaFile, error) {
	query := bson.M{
		"record_type":  scanmodel.RecordTypeMediaFile,
		"directory_id": directoryID,
	}
	findOptions := options.Find().SetSort(bson.D{{Key: "relative_path", Value: 1}})

	cursor, err := r.collection.Find(ctx, query, findOptions)
	if err != nil {
		return nil, fmt.Errorf("mongostore: listing media files: %w", err)
	}

	mediaFiles := []scanmodel.MediaFile{}
	if err := cursor.All(ctx, &mediaFiles); err != nil {
		return nil, fmt.Errorf("mongostore: decoding media files: %w", err)
	}
	return mediaFiles, nil
}

// GetMediaFile fetches one media file record by id.
func (r *LocalDirectoryRepo) GetMediaFile(ctx context.Context, id bson.ObjectID) (*scanmodel.MediaFile, error) {
	query := bson.M{"_id": id, "record_type": scanmodel.RecordTypeMediaFile}

	var mediaFile scanmodel.MediaFile
	err := r.collection.FindOne(ctx, query).Decode(&mediaFile)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mongostore: fetching media file %s: %w", id.Hex(), err)
	}
	return &mediaFile, nil
}

// FindByExternalLink returns the directories carrying a given provider id, e.g.
// key "tvdb" and value "81189".
func (r *LocalDirectoryRepo) FindByExternalLink(ctx context.Context, key, value string) ([]scanmodel.TVSeries, error) {
	query := bson.M{
		"record_type": scanmodel.RecordTypeTVSeries,
		"metadata.external_links": bson.M{
			"$elemMatch": bson.M{"key": key, "value": value},
		},
	}

	cursor, err := r.collection.Find(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("mongostore: finding directories by external link: %w", err)
	}

	directories := []scanmodel.TVSeries{}
	if err := cursor.All(ctx, &directories); err != nil {
		return nil, fmt.Errorf("mongostore: decoding directories: %w", err)
	}
	return directories, nil
}

// FindByEpisodeID returns the media files carrying a given episode-level
// provider id.
//
// It reads the same metadata.external_links field FindByExternalLink does; the
// record type is what separates them. An episode's ids come from its own NFO and
// describe the episode, so restricting to media file records is what makes this
// an episode lookup rather than a series one.
func (r *LocalDirectoryRepo) FindByEpisodeID(ctx context.Context, key, value string) ([]scanmodel.MediaFile, error) {
	query := bson.M{
		"record_type": scanmodel.RecordTypeMediaFile,
		"metadata.external_links": bson.M{
			"$elemMatch": bson.M{"key": key, "value": value},
		},
	}

	cursor, err := r.collection.Find(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("mongostore: finding media files by episode id: %w", err)
	}

	mediaFiles := []scanmodel.MediaFile{}
	if err := cursor.All(ctx, &mediaFiles); err != nil {
		return nil, fmt.Errorf("mongostore: decoding media files: %w", err)
	}
	return mediaFiles, nil
}
