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
// metarrv1connect.WorkflowCatalogServiceHandler, ported directly from
// internal/server/handlers/workflow_catalog.go. The catalog and the graph
// both cross the wire as opaque JSON bytes rather than modeled proto
// messages — see workflow_catalog.proto's doc comments for why.
type WorkflowCatalogServer struct {
	*handlers.Handlers
}

// WorkflowCatalogAuthPolicies is this service's method-name -> policy map.
// Mirrors the catalog/validate routes in router.go being GroupTasks.
var WorkflowCatalogAuthPolicies = map[string]httpserver.RPCPolicy{
	"Get":      {Group: auth.GroupTasks, ReadOnly: true},
	"Validate": {Group: auth.GroupTasks},
}

// catalogResponse mirrors handlers.CatalogResponse — kept private here since
// it exists only to be marshaled into the opaque catalog_json field.
type catalogResponse struct {
	NodeTypes     []workflow.NodeType  `json:"node_types"`
	Transforms    []workflow.Transform `json:"transforms"`
	SchemaVersion int                  `json:"schema_version"`
}

func (s *WorkflowCatalogServer) Get(
	ctx context.Context,
	req *connect.Request[metarrv1.WorkflowCatalogServiceGetRequest],
) (*connect.Response[metarrv1.WorkflowCatalogServiceGetResponse], error) {
	if s.WorkflowCatalog == nil {
		s.Logger.Error("workflow catalog is not loaded")
		return nil, connectError(http.StatusInternalServerError, errors.New("workflow catalog is not available"))
	}

	catalogJSON, err := json.Marshal(catalogResponse{
		NodeTypes:     s.WorkflowCatalog.All(),
		Transforms:    workflow.Transforms(),
		SchemaVersion: workflow.SchemaVersion,
	})
	if err != nil {
		s.Logger.Error("failed to encode workflow catalog", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to encode workflow catalog"))
	}

	return connect.NewResponse(&metarrv1.WorkflowCatalogServiceGetResponse{CatalogJson: catalogJSON}), nil
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
