package services

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/types/known/timestamppb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/server/mongostore/versioned"
)

const (
	defaultWorkflowLimit = 20
	maxWorkflowLimit     = 100
)

// WorkflowServer implements metarrv1connect.WorkflowServiceHandler, ported
// directly from internal/server/handlers/workflows.go — same Mongo reads
// and the same synchronous Save call (no system_config_update event; see
// UpsertWorkflow's doc comment), only the transport changed.
type WorkflowServer struct {
	*handlers.Handlers
}

// WorkflowAuthPolicies is this service's method-name -> policy map. Mirrors
// every workflow route in router.go being GroupTasks.
var WorkflowAuthPolicies = map[string]httpserver.RPCPolicy{
	"List":         {Group: auth.GroupTasks, ReadOnly: true},
	"Get":          {Group: auth.GroupTasks, ReadOnly: true},
	"ListVersions": {Group: auth.GroupTasks, ReadOnly: true},
	"GetVersion":   {Group: auth.GroupTasks, ReadOnly: true},
	"Upsert":       {Group: auth.GroupTasks},
}

func (s *WorkflowServer) List(
	ctx context.Context,
	req *connect.Request[metarrv1.WorkflowServiceListRequest],
) (*connect.Response[metarrv1.WorkflowServiceListResponse], error) {
	filter := versioned.LatestFilter{Limit: defaultWorkflowLimit}

	if limit := req.Msg.GetLimit(); limit != 0 {
		if limit < 1 {
			return nil, connectError(http.StatusBadRequest, errors.New("limit must be a positive integer"))
		}
		if limit > maxWorkflowLimit {
			limit = maxWorkflowLimit
		}
		filter.Limit = int64(limit)
	}

	if rawCursor := req.Msg.GetCursor(); rawCursor != "" {
		cursor, err := bson.ObjectIDFromHex(rawCursor)
		if err != nil {
			return nil, connectError(http.StatusBadRequest, errors.New("malformed cursor"))
		}
		filter.Cursor = cursor
	}

	workflows, nextCursor, hasMore, err := s.WorkflowRepo.ListLatest(ctx, filter)
	if err != nil {
		s.Logger.Error("failed to list workflows", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to list workflows"))
	}

	protoWorkflows := make([]*metarrv1.Workflow, 0, len(workflows))
	for _, w := range workflows {
		proto, err := workflowToProto(w)
		if err != nil {
			s.Logger.Error("failed to encode workflow", "error", err)
			return nil, connectError(http.StatusInternalServerError, errors.New("failed to encode workflow"))
		}
		protoWorkflows = append(protoWorkflows, proto)
	}

	return connect.NewResponse(&metarrv1.WorkflowServiceListResponse{
		Workflows:  protoWorkflows,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}), nil
}

func (s *WorkflowServer) Get(
	ctx context.Context,
	req *connect.Request[metarrv1.WorkflowServiceGetRequest],
) (*connect.Response[metarrv1.WorkflowServiceGetResponse], error) {
	workflowID, err := parseRecordID(req.Msg.GetId())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	workflow, err := s.WorkflowRepo.GetLatest(ctx, workflowID)
	if err != nil {
		return nil, workflowLookupError(err, workflowID, s.Logger)
	}

	proto, err := workflowToProto(workflow)
	if err != nil {
		s.Logger.Error("failed to encode workflow", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to encode workflow"))
	}

	return connect.NewResponse(&metarrv1.WorkflowServiceGetResponse{Workflow: proto}), nil
}

func (s *WorkflowServer) ListVersions(
	ctx context.Context,
	req *connect.Request[metarrv1.WorkflowServiceListVersionsRequest],
) (*connect.Response[metarrv1.WorkflowServiceListVersionsResponse], error) {
	workflowID, err := parseRecordID(req.Msg.GetId())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	versions, err := s.WorkflowRepo.ListVersions(ctx, workflowID)
	if err != nil {
		s.Logger.Error("failed to list workflow versions", "workflow_id", workflowID.Hex(), "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to list workflow versions"))
	}

	protoVersions := make([]*metarrv1.Workflow, 0, len(versions))
	for _, w := range versions {
		proto, err := workflowToProto(w)
		if err != nil {
			s.Logger.Error("failed to encode workflow", "error", err)
			return nil, connectError(http.StatusInternalServerError, errors.New("failed to encode workflow"))
		}
		protoVersions = append(protoVersions, proto)
	}

	return connect.NewResponse(&metarrv1.WorkflowServiceListVersionsResponse{Versions: protoVersions}), nil
}

func (s *WorkflowServer) GetVersion(
	ctx context.Context,
	req *connect.Request[metarrv1.WorkflowServiceGetVersionRequest],
) (*connect.Response[metarrv1.WorkflowServiceGetVersionResponse], error) {
	workflowID, err := parseRecordID(req.Msg.GetId())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	version := int(req.Msg.GetVersion())
	if version < 1 {
		return nil, connectError(http.StatusBadRequest, errors.New("version must be a positive integer"))
	}

	workflow, err := s.WorkflowRepo.GetVersion(ctx, workflowID, version)
	if err != nil {
		return nil, workflowLookupError(err, workflowID, s.Logger)
	}

	proto, err := workflowToProto(workflow)
	if err != nil {
		s.Logger.Error("failed to encode workflow", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to encode workflow"))
	}

	return connect.NewResponse(&metarrv1.WorkflowServiceGetVersionResponse{Workflow: proto}), nil
}

func (s *WorkflowServer) Upsert(
	ctx context.Context,
	req *connect.Request[metarrv1.WorkflowServiceUpsertRequest],
) (*connect.Response[metarrv1.WorkflowServiceUpsertResponse], error) {
	if req.Msg.GetName() == "" || req.Msg.GetDescription() == "" || len(req.Msg.GetTags()) == 0 {
		return nil, connectError(http.StatusBadRequest, errors.New("name, description and at least one tag are required"))
	}

	var documentID bson.ObjectID
	if raw := req.Msg.GetDocumentId(); raw != "" {
		id, err := bson.ObjectIDFromHex(raw)
		if err != nil {
			return nil, connectError(http.StatusBadRequest, errors.New("malformed document_id"))
		}
		documentID = id
	}

	entry, err := workflowFromUpsertProto(req.Msg)
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	saved, err := s.WorkflowRepo.Save(ctx, documentID, entry)
	if err != nil {
		s.Logger.Error("failed to save workflow", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to save workflow"))
	}

	proto, err := workflowToProto(saved)
	if err != nil {
		s.Logger.Error("failed to encode workflow", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to encode workflow"))
	}

	return connect.NewResponse(&metarrv1.WorkflowServiceUpsertResponse{Workflow: proto}), nil
}

// graphPayload is graph_json's decoded shape — exactly the three fields
// mongostore.Workflow stores as loose bson.M, bundled into the one opaque
// field every RPC here carries instead of modeling them.
type graphPayload struct {
	Nodes    []bson.M `json:"nodes"`
	Edges    []bson.M `json:"edges"`
	Viewport bson.M   `json:"viewport"`
}

func workflowToProto(w mongostore.Workflow) (*metarrv1.Workflow, error) {
	graphJSON, err := json.Marshal(graphPayload{Nodes: w.Nodes, Edges: w.Edges, Viewport: w.Viewport})
	if err != nil {
		return nil, err
	}
	return &metarrv1.Workflow{
		Id:            w.ID.Hex(),
		DocumentId:    w.DocumentID.Hex(),
		Version:       int32(w.Version),
		CreatedAt:     timestamppb.New(w.CreatedAt),
		Name:          w.Name,
		Description:   w.Description,
		Tags:          w.Tags,
		SchemaVersion: int32(w.SchemaVersion),
		GraphJson:     graphJSON,
	}, nil
}

func workflowFromUpsertProto(req *metarrv1.WorkflowServiceUpsertRequest) (mongostore.Workflow, error) {
	var graph graphPayload
	if err := json.Unmarshal(req.GetGraphJson(), &graph); err != nil {
		return mongostore.Workflow{}, errors.New("malformed graph")
	}
	return mongostore.Workflow{
		Name:          req.GetName(),
		Description:   req.GetDescription(),
		Tags:          req.GetTags(),
		SchemaVersion: int(req.GetSchemaVersion()),
		Nodes:         graph.Nodes,
		Edges:         graph.Edges,
		Viewport:      graph.Viewport,
	}, nil
}

func workflowLookupError(err error, workflowID bson.ObjectID, logger *slog.Logger) error {
	if errors.Is(err, versioned.ErrNotFound) {
		return connectError(http.StatusNotFound, errors.New("no workflow with that id"))
	}
	logger.Error("failed to fetch workflow", "workflow_id", workflowID.Hex(), "error", err)
	return connectError(http.StatusInternalServerError, errors.New("failed to fetch workflow"))
}
