package mongostore

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

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
	for _, mediaFile := range result.MediaFiles {
		// An unlinked media file (no directory record written yet) leaves
		// directory_id absent rather than storing a zero-value id string.
		if directoryID != bson.NilObjectID {
			mediaFile.DirectoryId = directoryID.Hex()
		}
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
	replacement, err := replacementDocumentFrom(directory)
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

	directory.Id = stored.ID.Hex()
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
	// scanned_at is stored as the protojson timestamp string (UTC RFC 3339),
	// so the cutoff is formatted the same way and compared lexicographically —
	// which is chronological order for these "…Z" strings.
	filter := bson.M{
		"scan_root_path": scanRootPath,
		"scanned_at":     bson.M{"$lt": scanmodel.FormatStoredTime(scanStartedAt)},
	}
	if _, err := r.collection.DeleteMany(ctx, filter); err != nil {
		return fmt.Errorf("mongostore: removing stale records under %s: %w", scanRootPath, err)
	}
	return nil
}

// replacementDocumentFrom encodes record as a whole-document replacement.
//
// The record is a generated message, so it is serialized through
// scanmodel.MarshalStored (protojson with proto field names) and the resulting
// JSON expanded into a BSON subdocument — not stored as an opaque blob — so the
// stored field names stay snake_case and the indexes keep matching. See
// docs/adr/0005.
//
// A replacement rather than a $set update is essential here, not a style
// preference. Unpopulated fields are not emitted, so a $set built from a rescan
// simply would not mention a field that had become empty — and MongoDB would
// leave the previous value in place. Warnings that had been fixed, metadata from
// a deleted NFO, subtitles that are no longer on disk: all of it would survive
// forever, which is the opposite of the rebuildable-cache behaviour this
// collection is meant to have. Replacing the document makes absent fields
// actually absent.
//
// A record's own identity is the collection's _id, a storage concern the
// message does not carry. The message's id field (empty on a fresh scan, the
// _id hex once a record has been read back) is dropped here too, so the stored
// document names itself only through _id: with _id absent from the replacement
// MongoDB keeps the existing one when replacing and mints a new one when
// inserting, which is what keeps ids stable across rescans. directory_id is a
// link between records rather than identity, so it stays.
func replacementDocumentFrom(record proto.Message) (bson.M, error) {
	storedJSON, err := scanmodel.MarshalStored(record)
	if err != nil {
		return nil, err
	}

	var fields bson.M
	if err := bson.UnmarshalExtJSON(storedJSON, false, &fields); err != nil {
		return nil, err
	}
	delete(fields, "_id")
	delete(fields, "id")

	// protojson renders 64-bit integers as quoted strings (size_bytes, and every
	// stat.* count), which would land in Mongo as strings — unsortable and
	// unqueryable as numbers. Put them back to int64, guided by the message
	// descriptor so the set is not a hand-maintained list.
	coerceStoredNumbers(fields, record.ProtoReflect().Descriptor())

	// The stale sweep range-queries scanned_at as a string, which is only
	// correct if every stored value has the same fixed-width shape — protojson's
	// own timestamp form does not. Normalize it on the way in; the nested
	// stat/*_at timestamps are never range-queried and keep protojson's form.
	if scannedAt, ok := fields["scanned_at"].(string); ok && scannedAt != "" {
		fields["scanned_at"] = scanmodel.NormalizeStoredTime(scannedAt)
	}

	return fields, nil
}

// coerceStoredNumbers walks node against md and converts any value protojson
// wrote as a quoted 64-bit integer back to int64, recursing into nested and
// repeated messages. bson.UnmarshalExtJSON nests documents as bson.D and the
// top level as bson.M, so both are handled. A 64-bit message field serialized
// as a scalar (google.protobuf.Timestamp is a string) is simply not a document
// and falls through untouched.
func coerceStoredNumbers(node any, md protoreflect.MessageDescriptor) {
	fields := md.Fields()

	coerce := func(key string, value any, set func(any)) {
		fd := fields.ByName(protoreflect.Name(key))
		if fd == nil {
			return
		}
		if fd.IsList() {
			if list, ok := value.(bson.A); ok {
				for i := range list {
					list[i] = coerceStoredValue(list[i], fd)
				}
			}
			return
		}
		set(coerceStoredValue(value, fd))
	}

	switch doc := node.(type) {
	case bson.M:
		for key, value := range doc {
			coerce(key, value, func(v any) { doc[key] = v })
		}
	case bson.D:
		for i := range doc {
			coerce(doc[i].Key, doc[i].Value, func(v any) { doc[i].Value = v })
		}
	}
}

func coerceStoredValue(value any, fd protoreflect.FieldDescriptor) any {
	switch fd.Kind() {
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		if s, ok := value.(string); ok {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return n
			}
		}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		if s, ok := value.(string); ok {
			// BSON has no uint64; store as int64, the same as a uint64 struct
			// field would encode. These fields (inode, link_count, device_id)
			// never approach int64's ceiling in practice.
			if n, err := strconv.ParseUint(s, 10, 64); err == nil {
				return int64(n)
			}
		}
	case protoreflect.MessageKind, protoreflect.GroupKind:
		coerceStoredNumbers(value, fd.Message())
	}
	return value
}

// decodeStoredRecord expands one stored document back into record and stamps the
// document's _id onto the message's own id field — the _id is a storage concern
// the message does not otherwise carry.
func decodeStoredRecord(row bson.M, record proto.Message) error {
	id, _ := row["_id"].(bson.ObjectID)
	delete(row, "_id")

	storedJSON, err := bson.MarshalExtJSON(row, false, false)
	if err != nil {
		return err
	}
	if err := scanmodel.UnmarshalStored(storedJSON, record); err != nil {
		return err
	}

	if id != bson.NilObjectID {
		if fd := record.ProtoReflect().Descriptor().Fields().ByName("id"); fd != nil && fd.Kind() == protoreflect.StringKind {
			record.ProtoReflect().Set(fd, protoreflect.ValueOfString(id.Hex()))
		}
	}
	return nil
}

// decodeStoredAll drains cursor into a slice of freshly decoded records. what
// names the record kind for the error message.
func decodeStoredAll[T any, PT interface {
	*T
	proto.Message
}](ctx context.Context, cursor *mongo.Cursor, what string) ([]PT, error) {
	var rows []bson.M
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("mongostore: decoding %s: %w", what, err)
	}

	records := make([]PT, 0, len(rows))
	for _, row := range rows {
		record := PT(new(T))
		if err := decodeStoredRecord(row, record); err != nil {
			return nil, fmt.Errorf("mongostore: decoding %s: %w", what, err)
		}
		records = append(records, record)
	}
	return records, nil
}

// ListFilter narrows a directory listing.
type ListFilter struct {
	ScanRootPath string
	Type         scanmodel.DirectoryType
	Limit        int64
	Skip         int64
}

// ListDirectories returns directory records matching filter, ordered by path.
func (r *LocalDirectoryRepo) ListDirectories(ctx context.Context, filter ListFilter) ([]*scanmodel.TVSeries, error) {
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
	return decodeStoredAll[scanmodel.TVSeries](ctx, cursor, "directories")
}

// GetDirectory fetches one directory record by id.
func (r *LocalDirectoryRepo) GetDirectory(ctx context.Context, id bson.ObjectID) (*scanmodel.TVSeries, error) {
	query := bson.M{"_id": id, "record_type": scanmodel.RecordTypeTVSeries}

	var row bson.M
	err := r.collection.FindOne(ctx, query).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mongostore: fetching directory %s: %w", id.Hex(), err)
	}

	var directory scanmodel.TVSeries
	if err := decodeStoredRecord(row, &directory); err != nil {
		return nil, fmt.Errorf("mongostore: decoding directory %s: %w", id.Hex(), err)
	}
	return &directory, nil
}

// ListMediaFiles returns the media file records belonging to one directory.
func (r *LocalDirectoryRepo) ListMediaFiles(ctx context.Context, directoryID bson.ObjectID) ([]*scanmodel.MediaFile, error) {
	query := bson.M{
		"record_type":  scanmodel.RecordTypeMediaFile,
		"directory_id": directoryID.Hex(),
	}
	findOptions := options.Find().SetSort(bson.D{{Key: "relative_path", Value: 1}})

	cursor, err := r.collection.Find(ctx, query, findOptions)
	if err != nil {
		return nil, fmt.Errorf("mongostore: listing media files: %w", err)
	}
	return decodeStoredAll[scanmodel.MediaFile](ctx, cursor, "media files")
}

// GetMediaFile fetches one media file record by id.
func (r *LocalDirectoryRepo) GetMediaFile(ctx context.Context, id bson.ObjectID) (*scanmodel.MediaFile, error) {
	query := bson.M{"_id": id, "record_type": scanmodel.RecordTypeMediaFile}

	var row bson.M
	err := r.collection.FindOne(ctx, query).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mongostore: fetching media file %s: %w", id.Hex(), err)
	}

	var mediaFile scanmodel.MediaFile
	if err := decodeStoredRecord(row, &mediaFile); err != nil {
		return nil, fmt.Errorf("mongostore: decoding media file %s: %w", id.Hex(), err)
	}
	return &mediaFile, nil
}

// FindByExternalLink returns the directories carrying a given provider id, e.g.
// key "tvdb" and value "81189".
func (r *LocalDirectoryRepo) FindByExternalLink(ctx context.Context, key, value string) ([]*scanmodel.TVSeries, error) {
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
	return decodeStoredAll[scanmodel.TVSeries](ctx, cursor, "directories")
}

// FindByEpisodeID returns the media files carrying a given episode-level
// provider id.
//
// It reads the same metadata.external_links field FindByExternalLink does; the
// record type is what separates them. An episode's ids come from its own NFO and
// describe the episode, so restricting to media file records is what makes this
// an episode lookup rather than a series one.
func (r *LocalDirectoryRepo) FindByEpisodeID(ctx context.Context, key, value string) ([]*scanmodel.MediaFile, error) {
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
	return decodeStoredAll[scanmodel.MediaFile](ctx, cursor, "media files")
}
