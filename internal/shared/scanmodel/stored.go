package scanmodel

import (
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// storedMarshal encodes a scan record the way it is both stored and served:
// proto field names, so the stored field names stay snake_case and the
// document is readable directly in the collection with the names the indexes
// use (record_type, directory_id, scan_root_path, scanned_at,
// metadata.external_links.*).
//
// Unpopulated fields are not emitted. Unlike the application config document —
// which lists every setting on purpose — the local_directory collection is a
// rebuildable cache where an absent field genuinely means absent, and records
// are written as whole-document replacements, so there is nothing to gain from
// spelling out every zero value.
var storedMarshal = protojson.MarshalOptions{UseProtoNames: true}

// storedUnmarshal is the matching decoder. DiscardUnknown keeps a document
// written by a newer build loadable by an older one.
var storedUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

// MarshalStored encodes record as the canonical stored/wire JSON form. It is
// the one place a scan record's serialization is defined, shared by the Mongo
// repository and any other reader.
func MarshalStored(record proto.Message) ([]byte, error) {
	return storedMarshal.Marshal(record)
}

// UnmarshalStored decodes bytes produced by MarshalStored (or any protojson
// encoding) into record.
func UnmarshalStored(data []byte, record proto.Message) error {
	return storedUnmarshal.Unmarshal(data, record)
}

// storedTimeLayout is a fixed-width UTC RFC 3339 form: always exactly nine
// fractional digits and a "Z" suffix, so every character position is a
// zero-padded field of constant width. Byte-for-byte string comparison of two
// such values is therefore the same as comparing the instants — which is what
// the stale sweep's "$lt scanned_at" relies on.
//
// protojson's own timestamp form varies between 0, 3, 6 and 9 fractional
// digits, and across that variation lexicographic order is *not* chronological
// ("...05.5Z" sorts before "...05Z" because '.' < 'Z'), so scanned_at is
// re-formatted to this layout before it is stored — see NormalizeStoredTime.
const storedTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// FormatStoredTime renders t in the fixed-width form scanned_at is stored in,
// for use as the cutoff in the stale sweep's range query.
func FormatStoredTime(t time.Time) string {
	return t.UTC().Format(storedTimeLayout)
}

// NormalizeStoredTime rewrites a protojson timestamp string into the
// fixed-width storedTimeLayout. A value that does not parse is returned
// unchanged.
func NormalizeStoredTime(protojsonTime string) string {
	parsed, err := time.Parse(time.RFC3339Nano, protojsonTime)
	if err != nil {
		return protojsonTime
	}
	return FormatStoredTime(parsed)
}
