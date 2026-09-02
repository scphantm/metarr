package aip

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// derivedFieldNames are the fields Normalize() backfills on read and that
// must never reach Mongo or the system_config_update payload. Today that is
// only the AIP resource name; the live agent-presence fields join this set
// when AgentService gains them (ADR-0010 storage carve-out).
var derivedFieldNames = []protoreflect.Name{"name"}

// ClearDerived strips every derived field (see derivedFieldNames) from each
// message, in place. It is meant to run inside a config-store Mutate closure
// on every resource the write touched, just before the document is
// marshalled, so nothing Normalize() added on read is persisted. A nil
// message is skipped.
func ClearDerived(messages ...proto.Message) {
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		reflectMsg := msg.ProtoReflect()
		if !reflectMsg.IsValid() {
			continue
		}
		fields := reflectMsg.Descriptor().Fields()
		for _, fieldName := range derivedFieldNames {
			if fd := fields.ByName(fieldName); fd != nil {
				reflectMsg.Clear(fd)
			}
		}
	}
}

// ClearDerivedSlice is ClearDerived over a homogeneous slice of resources —
// the shape a section-level write (sidecar types, Sonarr instances) carries.
func ClearDerivedSlice[T proto.Message](messages []T) {
	for _, msg := range messages {
		ClearDerived(msg)
	}
}
