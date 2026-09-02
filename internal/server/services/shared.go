package services

import (
	"google.golang.org/protobuf/proto"
)

// cloneMsg deep-copies m. The config model types are generated messages
// shared verbatim between the config store and the wire, so there is no
// conversion layer to cross — but a read that lifts a value out of live
// config into an RPC response, and a mutation that keeps a value from an
// RPC request in the persisted config, must clone rather than alias a
// pointer the other side owns. A nil m clones to a typed nil; callers that
// can receive one guard for it.
func cloneMsg[T proto.Message](m T) T {
	return proto.Clone(m).(T)
}
