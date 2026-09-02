package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
)

// operationNamePrefix is the single collection every config operation is
// addressed under (`operations/{correlation_id}`).
const operationNamePrefix = "operations/"

// defaultOperationPageSize / maxOperationPageSize bound ListOperations
// (AIP-158). The collection is TTL-reaped and tiny, so these are generous.
const (
	defaultOperationPageSize = 50
	maxOperationPageSize     = 200
)

// OperationsServer implements metarrv1connect.OperationsServiceHandler: the
// read side of the config API's long-running operations (ADR-0010). Writes are
// recorded by the config-mutating RPCs and finished by the
// system_config_update listener; this service only reads them back.
type OperationsServer struct {
	*handlers.Handlers
}

// OperationsAuthPolicies mirrors every other config service: GroupConfig,
// read-only — both methods are reads.
var OperationsAuthPolicies = map[string]httpserver.RPCPolicy{
	"GetOperation":   {Group: auth.GroupConfig, ReadOnly: true},
	"ListOperations": {Group: auth.GroupConfig, ReadOnly: true},
}

func (s *OperationsServer) GetOperation(
	ctx context.Context,
	req *connect.Request[metarrv1.GetOperationRequest],
) (*connect.Response[metarrv1.Operation], error) {
	name := req.Msg.GetName()
	if !strings.HasPrefix(name, operationNamePrefix) || name == operationNamePrefix {
		return nil, connectError(http.StatusBadRequest, fmt.Errorf("name must be %s{id}", operationNamePrefix))
	}

	op, err := s.Operations.Get(ctx, name)
	if err != nil {
		s.Logger.Error("failed to read operation", "name", name, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to read operation"))
	}
	if op == nil {
		return nil, connectError(http.StatusNotFound, fmt.Errorf("operation %q not found", name))
	}
	return connect.NewResponse(op), nil
}

func (s *OperationsServer) ListOperations(
	ctx context.Context,
	req *connect.Request[metarrv1.ListOperationsRequest],
) (*connect.Response[metarrv1.ListOperationsResponse], error) {
	done, err := parseDoneFilter(req.Msg.GetFilter())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	pageSize := int(req.Msg.GetPageSize())
	switch {
	case pageSize <= 0:
		pageSize = defaultOperationPageSize
	case pageSize > maxOperationPageSize:
		pageSize = maxOperationPageSize
	}

	ops, err := s.Operations.List(ctx, done, int64(pageSize))
	if err != nil {
		s.Logger.Error("failed to list operations", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to list operations"))
	}
	// The store is TTL-bounded and returns at most pageSize rows, so a
	// single page always holds the whole result — no next_page_token.
	return connect.NewResponse(&metarrv1.ListOperationsResponse{Operations: ops}), nil
}

// parseDoneFilter honours the one AIP-160 expression this service supports:
// `done = true` / `done = false` (spaces optional). An empty filter matches
// everything; anything else is InvalidArgument.
func parseDoneFilter(filter string) (*bool, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(filter), " ", "")
	switch normalized {
	case "":
		return nil, nil
	case "done=true":
		v := true
		return &v, nil
	case "done=false":
		v := false
		return &v, nil
	default:
		return nil, fmt.Errorf("filter must be empty, %q, or %q", "done = true", "done = false")
	}
}
