package agentproto

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// The agent config projection and presence record are generated messages
// written to Redis by one side of the contract and read by the other. They
// are serialized with protojson using proto field names — so the stored
// values keep snake_case keys and RFC 3339 timestamps and are readable
// directly with redis-cli — the same contract appconfig.MarshalStored uses
// for the config document. See docs/adr/0005.
var (
	storedMarshal   = protojson.MarshalOptions{UseProtoNames: true}
	storedUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}
)

// MarshalStored encodes a contract message as the JSON form stored in Redis.
func MarshalStored(message proto.Message) ([]byte, error) {
	return storedMarshal.Marshal(message)
}

// UnmarshalStored decodes bytes produced by MarshalStored into message.
func UnmarshalStored(data []byte, message proto.Message) error {
	return storedUnmarshal.Unmarshal(data, message)
}
