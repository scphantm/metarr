package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/server/workflow/validate"
	"Metarr/internal/shared/workflow"
)

// WorkflowCatalogServer implements
// metarrv1connect.WorkflowCatalogServiceHandler. The catalog crosses the wire
// as a typed WorkflowCatalog message now (docs/adr/0005); the graph is still
// carried as opaque JSON bytes on Validate — see workflow_catalog.proto.
type WorkflowCatalogServer struct {
	*handlers.Handlers
}

// WorkflowCatalogAuthPolicies is this service's method-name -> policy map.
// Mirrors the catalog/validate routes in router.go being GroupTasks.
var WorkflowCatalogAuthPolicies = map[string]httpserver.RPCPolicy{
	"Get":      {Group: auth.GroupTasks, ReadOnly: true},
	"Validate": {Group: auth.GroupTasks},
}

func (s *WorkflowCatalogServer) Get(
	ctx context.Context,
	req *connect.Request[metarrv1.WorkflowCatalogServiceGetRequest],
) (*connect.Response[metarrv1.WorkflowCatalogServiceGetResponse], error) {
	if s.WorkflowCatalog == nil {
		s.Logger.Error("workflow catalog is not loaded")
		return nil, connectError(http.StatusInternalServerError, errors.New("workflow catalog is not available"))
	}

	return connect.NewResponse(&metarrv1.WorkflowCatalogServiceGetResponse{
		Catalog: &metarrv1.WorkflowCatalog{
			NodeTypes:     s.WorkflowCatalog.All(),
			Transforms:    workflow.Transforms(),
			SchemaVersion: workflow.SchemaVersion,
		},
	}), nil
}

func (s *WorkflowCatalogServer) Validate(
	ctx context.Context,
	req *connect.Request[metarrv1.WorkflowCatalogServiceValidateRequest],
) (*connect.Response[metarrv1.WorkflowCatalogServiceValidateResponse], error) {
	if s.WorkflowCatalog == nil {
		s.Logger.Error("workflow catalog is not loaded")
		return nil, connectError(http.StatusInternalServerError, errors.New("workflow catalog is not available"))
	}

	var graph workflow.Graph
	if err := json.Unmarshal(req.Msg.GetGraphJson(), &graph); err != nil {
		return nil, connectError(http.StatusBadRequest, errors.New("malformed graph"))
	}

	result := validate.Graph(graph, s.WorkflowCatalog)

	diagnostics := make([]*metarrv1.Diagnostic, 0, len(result.Diagnostics))
	for _, d := range result.Diagnostics {
		diagnostics = append(diagnostics, &metarrv1.Diagnostic{
			Severity:    string(d.Severity),
			Code:        d.Code,
			Message:     d.Message,
			NodeIds:     d.NodeIDs,
			EdgeIds:     d.EdgeIDs,
			WitnessPath: d.WitnessPath,
		})
	}

	return connect.NewResponse(&metarrv1.WorkflowCatalogServiceValidateResponse{
		Diagnostics: diagnostics,
		Runnable:    result.Runnable(),
	}), nil
}
