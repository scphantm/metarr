package services

import (
	"google.golang.org/protobuf/proto"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/shared/eventbus"
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

// acceptedResponse builds the shared "queued, not yet persisted" response
// every config-mutation RPC returns after a successful FireConfigUpdate —
// the gRPC-Web equivalent of the REST handlers' 202 AcceptedResponse body.
func acceptedResponse(correlationID string) *metarrv1.AcceptedResponse {
	return &metarrv1.AcceptedResponse{
		Status:        "accepted",
		Event:         eventbus.SystemConfigUpdateEventName,
		CorrelationId: correlationID,
	}
}
