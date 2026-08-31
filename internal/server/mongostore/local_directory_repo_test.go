package mongostore

import (
	"bytes"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/shared/scanmodel"
)

// TestReplacementDocumentOmitsIdentity covers the invariant the id strategy
// rests on. A record names itself only through the collection's _id, which the
// message does not carry; with neither _id nor the message's own id field in
// the replacement, MongoDB keeps the existing _id when replacing and mints a
// new one when inserting — which is what makes ids stable across rescans.
func TestReplacementDocumentOmitsIdentity(t *testing.T) {
	tests := []struct {
		name   string
		record proto.Message
	}{
		{
			name: "freshly scanned directory",
			record: &scanmodel.TVSeries{
				RecordType: scanmodel.RecordTypeTVSeries,
				Path:       "/media/Movies/The Matrix (1999)",
				Type:       scanmodel.TypeMovie,
			},
		},
		{
			// A record read back from storage carries a real id; it must still
			// be dropped before the update is sent.
			name: "directory already carrying an id",
			record: &scanmodel.TVSeries{
				Id:         bson.NewObjectID().Hex(),
				RecordType: scanmodel.RecordTypeTVSeries,
				Path:       "/media/Movies/The Matrix (1999)",
			},
		},
		{
			name: "media file already carrying an id",
			record: &scanmodel.MediaFile{
				Id:          bson.NewObjectID().Hex(),
				DirectoryId: bson.NewObjectID().Hex(),
				RecordType:  scanmodel.RecordTypeMediaFile,
				Path:        "/media/Movies/The Matrix (1999)/The Matrix (1999).mkv",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields, err := replacementDocumentFrom(test.record)
			if err != nil {
				t.Fatalf("replacementDocumentFrom() error = %v", err)
			}

			if _, present := fields["_id"]; present {
				t.Errorf("replacement contains _id, which MongoDB will reject: %+v", fields)
			}
			if _, present := fields["id"]; present {
				t.Errorf("replacement contains the message's own id field; identity is the collection _id only: %+v", fields)
			}
			if fields["path"] == nil {
				t.Errorf("replacement is missing path, the natural key: %+v", fields)
			}
		})
	}
}

// TestReplacementIsNotAPartialUpdate is the regression guard for a bug that only
// showed up end to end: an empty field must be absent from the replacement so a
// whole-document overwrite genuinely clears it, rather than a $set that would
// leave the stale value in place. Warnings that had been fixed, metadata from a
// deleted NFO, subtitles no longer on disk — all of it would otherwise survive
// forever.
func TestReplacementIsNotAPartialUpdate(t *testing.T) {
	// A record whose optional fields are all empty, standing in for a rescan of
	// a directory whose warnings have been resolved.
	cleaned := &scanmodel.TVSeries{
		RecordType: scanmodel.RecordTypeTVSeries,
		Path:       "/media/Shows/Fixed Show (2010)",
		Type:       scanmodel.TypeTV,
		Sidecars:   []*scanmodel.SidecarFile{},
	}

	fields, err := replacementDocumentFrom(cleaned)
	if err != nil {
		t.Fatalf("replacementDocumentFrom() error = %v", err)
	}

	for _, operator := range []string{"$set", "$unset", "$setOnInsert"} {
		if _, present := fields[operator]; present {
			t.Fatalf("encoded form contains the %s operator; it must be a whole-document replacement so emptied fields are actually cleared: %+v", operator, fields)
		}
	}

	// The emptied fields must be absent from the replacement, which is precisely
	// why the replacement has to overwrite the whole document.
	for _, field := range []string{"warnings", "seasons", "sidecars", "metadata"} {
		if _, present := fields[field]; present {
			t.Errorf("expected %q to be absent from a cleaned record, got %#v", field, fields[field])
		}
	}

	// And the fields that do carry values must still be there.
	if fields["record_type"] == nil || fields["path"] == nil || fields["type"] == nil {
		t.Errorf("replacement lost a populated field: %+v", fields)
	}
}

// TestReplacementNormalizesScannedAt pins that scanned_at lands in storage as
// the fixed-width form the stale sweep range-queries against, not protojson's
// variable-precision form (which does not sort chronologically as a string).
func TestReplacementNormalizesScannedAt(t *testing.T) {
	fields, err := replacementDocumentFrom(&scanmodel.TVSeries{
		RecordType: scanmodel.RecordTypeTVSeries,
		Path:       "/media/Shows/S",
		ScannedAt:  timestamppb.New(time.Date(2026, 1, 2, 15, 4, 5, 500_000_000, time.UTC)),
	})
	if err != nil {
		t.Fatalf("replacementDocumentFrom() error = %v", err)
	}

	got, _ := fields["scanned_at"].(string)
	if want := "2026-01-02T15:04:05.500000000Z"; got != want {
		t.Errorf("scanned_at = %q, want the fixed-width form %q", got, want)
	}
}

// TestReplacementPreservesDirectoryLink checks that a media file's link to its
// directory survives encoding, since that link is what makes the two record
// kinds navigable. It is stored as the directory's _id hex.
func TestReplacementPreservesDirectoryLink(t *testing.T) {
	directoryID := bson.NewObjectID()
	mediaFile := &scanmodel.MediaFile{
		RecordType:  scanmodel.RecordTypeMediaFile,
		DirectoryId: directoryID.Hex(),
		Path:        "/media/Shows/Breaking Bad (2008)/Season 01/S01E01.mkv",
	}

	fields, err := replacementDocumentFrom(mediaFile)
	if err != nil {
		t.Fatalf("replacementDocumentFrom() error = %v", err)
	}

	storedID, ok := fields["directory_id"].(string)
	if !ok {
		t.Fatalf("directory_id missing or wrong type: %#v", fields["directory_id"])
	}
	if storedID != directoryID.Hex() {
		t.Errorf("directory_id = %v, want %v", storedID, directoryID.Hex())
	}
}

// TestReplacementDropsUnsetDirectoryID confirms an unset directory link is
// omitted rather than written as an empty string, which a listing would then
// have to distinguish from a real reference.
func TestReplacementDropsUnsetDirectoryID(t *testing.T) {
	fields, err := replacementDocumentFrom(&scanmodel.MediaFile{
		RecordType: scanmodel.RecordTypeMediaFile,
		Path:       "/media/x.mkv",
	})
	if err != nil {
		t.Fatalf("replacementDocumentFrom() error = %v", err)
	}

	if _, present := fields["directory_id"]; present {
		t.Errorf("unset directory_id should be omitted, got %#v", fields["directory_id"])
	}
}

// TestReplacementStoresLargeIntegersAsNumbers pins that 64-bit fields land in
// Mongo as numbers, not the quoted strings protojson emits — so a listing can
// sort or range-query on size_bytes, and the stat counts stay numeric too.
func TestReplacementStoresLargeIntegersAsNumbers(t *testing.T) {
	fields, err := replacementDocumentFrom(&scanmodel.MediaFile{
		RecordType: scanmodel.RecordTypeMediaFile,
		Path:       "/media/big.mkv",
		SizeBytes:  4 << 30, // 4 GiB, well beyond int32
		Stat: &scanmodel.FileStat{
			Inode:     999_999_999_999,
			LinkCount: 1,
			SizeBytes: 42,
			ModeBits:  0o644,
		},
	})
	if err != nil {
		t.Fatalf("replacementDocumentFrom() error = %v", err)
	}

	if got, ok := fields["size_bytes"].(int64); !ok || got != 4<<30 {
		t.Errorf("size_bytes = %#v, want int64 %d", fields["size_bytes"], int64(4<<30))
	}

	stat, ok := fields["stat"].(bson.D)
	if !ok {
		t.Fatalf("stat subdocument missing or wrong type: %#v", fields["stat"])
	}
	statField := func(key string) any {
		for _, e := range stat {
			if e.Key == key {
				return e.Value
			}
		}
		return nil
	}
	for _, key := range []string{"inode", "link_count", "size_bytes"} {
		if _, ok := statField(key).(int64); !ok {
			t.Errorf("stat.%s = %#v, want an int64", key, statField(key))
		}
	}
	// int32 fields were never affected and must not be touched.
	if _, ok := statField("mode_bits").(int32); !ok {
		t.Errorf("stat.mode_bits = %#v, want an int32", statField("mode_bits"))
	}
}

// TestStoredRoundTripKeepsSnakeCaseNames pins the storage contract the indexes
// depend on: a record encoded and read back is unchanged, and the stored field
// names are the snake_case ones an operator recognises — in particular the
// metadata.external_links path the provider-id index is built on.
func TestStoredRoundTripKeepsSnakeCaseNames(t *testing.T) {
	original := &scanmodel.TVSeries{
		RecordType:   scanmodel.RecordTypeTVSeries,
		Path:         "/media/Shows/Breaking Bad (2008)",
		ScanRootPath: "/media/Shows",
		Type:         scanmodel.TypeTV,
		FolderName:   "Breaking Bad (2008)",
		Metadata: &metarrv1.Metadata{
			ExternalLinks: []*metarrv1.Link{{Key: "tvdb", Value: "81189"}},
		},
		Sidecars: []*scanmodel.SidecarFile{
			{
				RelativePath: "poster.jpg", FileName: "poster.jpg",
				Type: scanmodel.SidecarPoster, Category: scanmodel.SidecarCategoryImage,
				SizeBytes: 5 << 20,
				Stat:      &scanmodel.FileStat{Inode: 12_884_901_888, LinkCount: 1, SizeBytes: 5 << 20},
			},
		},
	}

	fields, err := replacementDocumentFrom(original)
	if err != nil {
		t.Fatalf("replacementDocumentFrom() error = %v", err)
	}

	for _, name := range []string{"record_type", "scan_root_path", "folder_name"} {
		if _, present := fields[name]; !present {
			t.Errorf("stored document is missing snake_case field %q: %+v", name, fields)
		}
	}

	storedJSON, err := bson.MarshalExtJSON(fields, false, false)
	if err != nil {
		t.Fatalf("re-encoding the stored document: %v", err)
	}

	// The provider-id index is built on metadata.external_links.key/value, so
	// the nested field names must stay snake_case too.
	if !bytes.Contains(storedJSON, []byte(`"external_links"`)) {
		t.Errorf("metadata.external_links missing from the stored document: %s", storedJSON)
	}

	var readBack scanmodel.TVSeries
	if err := scanmodel.UnmarshalStored(storedJSON, &readBack); err != nil {
		t.Fatalf("UnmarshalStored() error = %v", err)
	}

	// id is stamped from the collection _id on read, not carried in the body.
	original.Id = ""
	if !proto.Equal(original, &readBack) {
		t.Errorf("round trip changed the record:\n got %v\nwant %v", &readBack, original)
	}
}
