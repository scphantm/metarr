package services

import (
	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/shared/eventbus"
)

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
