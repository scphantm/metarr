package services

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/encoding/protojson"
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

// WorkflowStore is the narrow, consumer-declared view of the versioned
// workflow repository that WorkflowServer needs — only the append-only save
// plus the four reads it calls today, no speculative surface. It is declared
// here, by the consumer, the same way appconfigstore declares its own
// configReader / configWriter interfaces rather than naming a concrete repo.
// The concrete *mongostore.WorkflowRepo satisfies it unchanged in
// production; an in-memory fake satisfies it in tests.
type WorkflowStore interface {
	Save(ctx context.Context, documentID bson.ObjectID, w mongostore.Workflow) (mongostore.Workflow, error)
	ListLatest(ctx context.Context, filter versioned.LatestFilter) ([]mongostore.Workflow, string, bool, error)
	GetLatest(ctx context.Context, documentID bson.ObjectID) (mongostore.Workflow, error)
	GetVersion(ctx context.Context, documentID bson.ObjectID, version int) (mongostore.Workflow, error)
	ListVersions(ctx context.Context, documentID bson.ObjectID) ([]mongostore.Workflow, error)
}

// WorkflowServer implements metarrv1connect.WorkflowServiceHandler. It reads
// and writes workflow graphs through the WorkflowStore seam — the append-only
// versioned store, reached via the concrete repo in production and a fake in
// tests. Writes are synchronous (see Upsert's doc comment).
type WorkflowServer struct {
	*handlers.Handlers

	// Store is the workflow persistence seam. The composition root
	// (cmd/metarr-server) wires the concrete *mongostore.WorkflowRepo here;
	// tests wire an in-memory fake.
	Store WorkflowStore
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

	workflows, nextCursor, hasMore, err := s.Store.ListLatest(ctx, filter)
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

	workflow, err := s.Store.GetLatest(ctx, workflowID)
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

	versions, err := s.Store.ListVersions(ctx, workflowID)
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

	workflow, err := s.Store.GetVersion(ctx, workflowID, version)
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

	saved, err := s.Store.Save(ctx, documentID, entry)
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

// The workflow graph is a generated message (workflow.Graph aliases
// metarr.v1.WorkflowGraph). It is persisted the same way the application
// config document is — protojson with proto field names, expanded into a
// BSON subdocument rather than an opaque blob — so the stored node/edge
// field names stay snake_case and the collection is still readable directly
// (docs/adr/0005). mongostore.Workflow keeps its three loose fields
// (Nodes/Edges/Viewport) so this is the only place the graph message is
// converted, and there is no per-node-type schema to keep in step.
//
// storedGraphUnmarshal is strict on purpose: an unrecognised node type and
// unrecognised settings ride through in the message's structured `type`,
// `settings` and `extra` fields, so a stored graph should never carry a key
// the message cannot place. If one appears, failing loudly beats silently
// dropping part of someone's workflow.
var (
	storedGraphMarshal   = protojson.MarshalOptions{UseProtoNames: true}
	storedGraphUnmarshal = protojson.UnmarshalOptions{}
)

// storedGraph is the graph's shape inside the workflow document.
type storedGraph struct {
	SchemaVersion int      `bson:"schema_version"`
	Nodes         []bson.M `bson:"nodes"`
	Edges         []bson.M `bson:"edges"`
	Viewport      bson.M   `bson:"viewport"`
}

func graphToProto(w mongostore.Workflow) (*metarrv1.WorkflowGraph, error) {
	doc := bson.M{
		"schema_version": w.SchemaVersion,
		"nodes":          w.Nodes,
		"edges":          w.Edges,
	}
	if w.Viewport != nil {
		doc["viewport"] = w.Viewport
	}
	extJSON, err := bson.MarshalExtJSON(doc, false, false)
	if err != nil {
		return nil, err
	}
	graph := &metarrv1.WorkflowGraph{}
	if err := storedGraphUnmarshal.Unmarshal(extJSON, graph); err != nil {
		return nil, err
	}
	return graph, nil
}

func graphFromProto(graph *metarrv1.WorkflowGraph) (storedGraph, error) {
	if graph == nil {
		graph = &metarrv1.WorkflowGraph{}
	}
	protoJSON, err := storedGraphMarshal.Marshal(graph)
	if err != nil {
		return storedGraph{}, err
	}
	var stored storedGraph
	if err := bson.UnmarshalExtJSON(protoJSON, false, &stored); err != nil {
		return storedGraph{}, err
	}
	return stored, nil
}

func workflowToProto(w mongostore.Workflow) (*metarrv1.Workflow, error) {
	graph, err := graphToProto(w)
	if err != nil {
		return nil, err
	}
	return &metarrv1.Workflow{
		Id:          w.ID.Hex(),
		DocumentId:  w.DocumentID.Hex(),
		Version:     int32(w.Version),
		CreatedAt:   timestamppb.New(w.CreatedAt),
		Name:        w.Name,
		Description: w.Description,
		Tags:        w.Tags,
		Graph:       graph,
	}, nil
}

func workflowFromUpsertProto(req *metarrv1.WorkflowServiceUpsertRequest) (mongostore.Workflow, error) {
	stored, err := graphFromProto(req.Graph)
	if err != nil {
		return mongostore.Workflow{}, errors.New("malformed graph")
	}
	return mongostore.Workflow{
		Name:          req.Name,
		Description:   req.Description,
		Tags:          req.Tags,
		SchemaVersion: stored.SchemaVersion,
		Nodes:         stored.Nodes,
		Edges:         stored.Edges,
		Viewport:      stored.Viewport,
	}, nil
}

func workflowLookupError(err error, workflowID bson.ObjectID, logger *slog.Logger) error {
	if errors.Is(err, versioned.ErrNotFound) {
		return connectError(http.StatusNotFound, errors.New("no workflow with that id"))
	}
	logger.Error("failed to fetch workflow", "workflow_id", workflowID.Hex(), "error", err)
	return connectError(http.StatusInternalServerError, errors.New("failed to fetch workflow"))
}
