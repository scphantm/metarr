package mongostore

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"Metarr/internal/mediascan"
)

// TestReplacementDocumentOmitsID covers the invariant the id strategy rests on.
// With _id absent, MongoDB keeps the existing id when replacing a document and
// assigns a new one when inserting — which is what makes ids stable across
// rescans. An _id present in the replacement would be rejected outright.
func TestReplacementDocumentOmitsID(t *testing.T) {
	tests := []struct {
		name   string
		record any
	}{
		{
			name: "freshly scanned directory",
			record: mediascan.TVSeries{
				RecordType: mediascan.RecordTypeTVSeries,
				Path:       "/media/Movies/The Matrix (1999)",
				Type:       mediascan.TypeMovie,
			},
		},
		{
			// A record that has already been through storage carries a real id;
			// it must still be stripped before the update is sent.
			name: "directory already carrying an id",
			record: mediascan.TVSeries{
				ID:         bson.NewObjectID(),
				RecordType: mediascan.RecordTypeTVSeries,
				Path:       "/media/Movies/The Matrix (1999)",
			},
		},
		{
			name: "media file already carrying an id",
			record: mediascan.MediaFile{
				ID:          bson.NewObjectID(),
				DirectoryID: bson.NewObjectID(),
				RecordType:  mediascan.RecordTypeMediaFile,
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
			if fields["path"] == nil {
				t.Errorf("replacement is missing path, the natural key: %+v", fields)
			}
		})
	}
}

// TestReplacementIsNotAPartialUpdate is the regression guard for a bug that only
// showed up end to end: most fields on these records are tagged omitempty, so a
// $set built from a rescan omits any field that has become empty and MongoDB
// keeps the stale value. Warnings that had been fixed, metadata from a deleted
// NFO, subtitles no longer on disk — all of it would survive forever.
//
// A whole-document replacement is what makes an absent field genuinely absent,
// so this asserts the encoded form really is the full document and not an update
// operator.
func TestReplacementIsNotAPartialUpdate(t *testing.T) {
	// A record whose omitempty fields are all empty, standing in for a rescan of
	// a directory whose warnings have been resolved.
	cleaned := mediascan.TVSeries{
		RecordType: mediascan.RecordTypeTVSeries,
		Path:       "/media/Shows/Fixed Show (2010)",
		Type:       mediascan.TypeTV,
		Sidecars:   []mediascan.SidecarFile{},
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
	for _, field := range []string{"warnings", "seasons", "year", "artist"} {
		if _, present := fields[field]; present {
			t.Errorf("expected %q to be absent from a cleaned record, got %#v", field, fields[field])
		}
	}

	// And the fields that do carry values must still be there.
	if fields["record_type"] == nil || fields["path"] == nil || fields["type"] == nil {
		t.Errorf("replacement lost a populated field: %+v", fields)
	}
}

// TestReplacementPreservesDirectoryLink checks that a media file's link to its
// directory survives encoding, since that link is what makes the two record
// kinds navigable.
func TestReplacementPreservesDirectoryLink(t *testing.T) {
	directoryID := bson.NewObjectID()
	mediaFile := mediascan.MediaFile{
		RecordType:  mediascan.RecordTypeMediaFile,
		DirectoryID: directoryID,
		Path:        "/media/Shows/Breaking Bad (2008)/Season 01/S01E01.mkv",
	}

	fields, err := replacementDocumentFrom(mediaFile)
	if err != nil {
		t.Fatalf("replacementDocumentFrom() error = %v", err)
	}

	storedID, ok := fields["directory_id"].(bson.ObjectID)
	if !ok {
		t.Fatalf("directory_id missing or wrong type: %#v", fields["directory_id"])
	}
	if storedID != directoryID {
		t.Errorf("directory_id = %v, want %v", storedID, directoryID)
	}
}

// TestReplacementDropsZeroDirectoryID confirms an unset directory link is
// omitted rather than written as a zero ObjectID, which would look like a real
// reference to a nonexistent directory.
func TestReplacementDropsZeroDirectoryID(t *testing.T) {
	fields, err := replacementDocumentFrom(mediascan.MediaFile{
		RecordType: mediascan.RecordTypeMediaFile,
		Path:       "/media/x.mkv",
	})
	if err != nil {
		t.Fatalf("replacementDocumentFrom() error = %v", err)
	}

	if _, present := fields["directory_id"]; present {
		t.Errorf("zero directory_id should be omitted, got %#v", fields["directory_id"])
	}
}
