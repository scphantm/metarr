package services

import (
	"context"
	"log/slog"

	"google.golang.org/protobuf/proto"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/handlers"
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

// acceptedResponse builds the shared "queued, not yet persisted" response the
// config-mutation RPCs that have not yet moved to AIP-151 operations return
// after a successful FireConfigUpdate — the gRPC-Web equivalent of the REST
// handlers' 202 AcceptedResponse body.
func acceptedResponse(correlationID string) *metarrv1.AcceptedResponse {
	return &metarrv1.AcceptedResponse{
		Status:        "accepted",
		Event:         eventbus.SystemConfigUpdateEventName,
		CorrelationId: correlationID,
	}
}

// beginConfigOperation records the AIP-151 operation a config write opens and
// returns the handle to hand back to the caller (name: operations/{cid}). The
// system_config_update listener finishes it. A failure to record here is
// logged, not surfaced: the write has already fired and the listener's
// Complete upserts, so the caller can still poll the name.
func beginConfigOperation(ctx context.Context, ops handlers.OperationStore, logger *slog.Logger, correlationID string) *metarrv1.Operation {
	name := operationNamePrefix + correlationID
	if err := ops.Create(ctx, name); err != nil {
		logger.Warn("failed to record config operation; the listener will still create it",
			"operation", name, "error", err)
	}
	return &metarrv1.Operation{Name: name, Done: false}
}
