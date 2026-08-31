package logging

import (
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"
)

// The canonical form of a log record on eventbus.LogChannel is a flat JSON
// object — time/level/message/source, plus a nested attrs object when the
// caller attached any. Fluent Bit consumes those bytes verbatim, so this
// package is careful about two things the generated message cannot preserve
// on its own:
//
//   - integer attribute values. google.protobuf.Struct holds every number as
//     a float64, which silently rounds an int64 past 2^53 (a UnixNano
//     timestamp, a large external id). marshalLogLine serialises the raw
//     attribute map with encoding/json instead, so those land exactly as
//     they always have.
//   - the always-present scalar keys. protojson omits an empty string field;
//     encoding/json on the map below always emits time/level/message/source,
//     matching the struct this record used to be.
//
// UnmarshalRecord is the read side — the live-tail buffer decodes the same
// bytes into the typed *LogRecord. Numbers become float64 there, which is
// inherent to google.protobuf.Struct and immaterial: nothing renders a
// record's attrs, only its time/level/source/message.

var recordUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

// marshalLogLine encodes one record as the flat JSON object published to
// eventbus.LogChannel. attrs may be nil, in which case no attrs key is
// emitted.
func marshalLogLine(time, level, message, source string, attrs map[string]any) ([]byte, error) {
	line := map[string]any{
		"time":    time,
		"level":   level,
		"message": message,
		"source":  source,
	}
	if len(attrs) > 0 {
		line["attrs"] = attrs
	}
	return json.Marshal(line)
}

// UnmarshalRecord decodes one record as published to eventbus.LogChannel. It
// is lenient about unknown fields so a record written by a newer build does
// not vanish from an older build's live tail.
func UnmarshalRecord(data []byte) (*Record, error) {
	record := &Record{}
	if err := recordUnmarshal.Unmarshal(data, record); err != nil {
		return nil, err
	}
	return record, nil
}
