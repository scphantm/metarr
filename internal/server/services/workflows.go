package services

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/server/mongostore/versioned"
)

const (
	defaultWorkflowPageSize = 50
	maxWorkflowPageSize     = 100
)

// workflowUpdatePaths is the closed set of update_mask paths UpdateWorkflow
// honours. graph is masked wholesale — a mask may name graph but not a
// sub-path — because the stored graph is an opaque document (docs/adr/0010).
var workflowUpdatePaths = map[string]bool{
	"name":        true,
	"description": true,
	"tags":        true,
	"graph":       true,
}

// WorkflowStore is the narrow, consumer-declared view of the versioned
// workflow repository that WorkflowServer needs — the append-only save, the
// four reads it calls, and the delete-all-versions the AIP DeleteWorkflow
// method added. It is declared here, by the consumer, the same way
// appconfigstore declares its own configReader / configWriter interfaces
// rather than naming a concrete repo. The concrete *mongostore.WorkflowRepo
// satisfies it unchanged in production; an in-memory fake satisfies it in
// tests.
type WorkflowStore interface {
	Save(ctx context.Context, documentID bson.ObjectID, w mongostore.Workflow) (mongostore.Workflow, error)
	ListLatest(ctx context.Context, filter versioned.LatestFilter) ([]mongostore.Workflow, string, bool, error)
	GetLatest(ctx context.Context, documentID bson.ObjectID) (mongostore.Workflow, error)
	GetVersion(ctx context.Context, documentID bson.ObjectID, version int) (mongostore.Workflow, error)
	ListVersions(ctx context.Context, documentID bson.ObjectID) ([]mongostore.Workflow, error)
	DeleteAllVersions(ctx context.Context, documentID bson.ObjectID) error
}

// WorkflowServer implements metarrv1connect.WorkflowServiceHandler on the AIP
// standard methods (docs/adr/0010). It reads and writes workflow graphs
// through the WorkflowStore seam — the append-only versioned store, reached
// via the concrete repo in production and a fake in tests. Writes are
// synchronous: CreateWorkflow / UpdateWorkflow append a version and return the
// stored resource, DeleteWorkflow removes every version and returns empty.
type WorkflowServer struct {
	*handlers.Handlers

	// Store is the workflow persistence seam. The composition root
	// (cmd/metarr-server) wires the concrete *mongostore.WorkflowRepo here;
	// tests wire an in-memory fake.
	Store WorkflowStore
}

// WorkflowAuthPolicies is this service's method-name -> policy map. Every
// method stays in the tasks group, mirroring the pre-AIP shape: the reads are
// read-only, the writes are gated.
var WorkflowAuthPolicies = map[string]httpserver.RPCPolicy{
	"CreateWorkflow":       {Group: auth.GroupTasks},
	"GetWorkflow":          {Group: auth.GroupTasks, ReadOnly: true},
	"ListWorkflows":        {Group: auth.GroupTasks, ReadOnly: true},
	"UpdateWorkflow":       {Group: auth.GroupTasks},
	"DeleteWorkflow":       {Group: auth.GroupTasks},
	"GetWorkflowVersion":   {Group: auth.GroupTasks, ReadOnly: true},
	"ListWorkflowVersions": {Group: auth.GroupTasks, ReadOnly: true},
}

func (s *WorkflowServer) CreateWorkflow(
	ctx context.Context,
	req *connect.Request[metarrv1.CreateWorkflowRequest],
) (*connect.Response[metarrv1.Workflow], error) {
	entry := req.Msg.GetWorkflow()
	if entry == nil {
		return nil, connectError(http.StatusBadRequest, errors.New("workflow is required"))
	}
	if entry.GetId() != "" {
		return nil, connectError(http.StatusBadRequest, errors.New("id is server-minted and must not be set on Create"))
	}
	if err := validateWorkflowContent(entry); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	stored, err := workflowFromProto(entry)
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	saved, err := s.Store.Save(ctx, bson.NilObjectID, stored)
	if err != nil {
		s.Logger.Error("failed to save workflow", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to save workflow"))
	}
	return s.encodeWorkflow(saved)
}

func (s *WorkflowServer) GetWorkflow(
	ctx context.Context,
	req *connect.Request[metarrv1.GetWorkflowRequest],
) (*connect.Response[metarrv1.Workflow], error) {
	workflowID, err := parseRecordID(req.Msg.GetId())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	workflow, err := s.Store.GetLatest(ctx, workflowID)
	if err != nil {
		return nil, workflowLookupError(err, workflowID, s.Logger)
	}
	return s.encodeWorkflow(workflow)
}

func (s *WorkflowServer) ListWorkflows(
	ctx context.Context,
	req *connect.Request[metarrv1.ListWorkflowsRequest],
) (*connect.Response[metarrv1.ListWorkflowsResponse], error) {
	if err := parseListFilter(req.Msg.GetFilter()); err != nil {
		return nil, aipConnectError(err)
	}
	// The list is storage-ordered newest-first and paged by an opaque _id
	// cursor, so an order_by cannot be applied across pages. It is
	// Unimplemented rather than silently ignored, matching how filter is
	// handled (docs/adr/0010).
	if req.Msg.GetOrderBy() != "" {
		return nil, connect.NewError(connect.CodeUnimplemented,
			errors.New("order_by is not supported: workflows are listed newest-first"))
	}

	filter := versioned.LatestFilter{Limit: defaultWorkflowPageSize}
	if pageSize := req.Msg.GetPageSize(); pageSize != 0 {
		if pageSize < 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("page_size must not be negative"))
		}
		if pageSize > maxWorkflowPageSize {
			pageSize = maxWorkflowPageSize
		}
		filter.Limit = int64(pageSize)
	}

	if rawToken := req.Msg.GetPageToken(); rawToken != "" {
		cursor, err := bson.ObjectIDFromHex(rawToken)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("malformed page_token"))
		}
		filter.Cursor = cursor
	}

	workflows, nextToken, _, err := s.Store.ListLatest(ctx, filter)
	if err != nil {
		s.Logger.Error("failed to list workflows", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to list workflows"))
	}

	protoWorkflows, err := s.encodeWorkflows(workflows)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&metarrv1.ListWorkflowsResponse{
		Workflows:     protoWorkflows,
		NextPageToken: nextToken,
	}), nil
}

// UpdateWorkflow is an AIP-134 partial update: read the latest version, apply
// the mask, append the merged result as a new immutable version. Allowed mask
// paths are name / description / tags / graph (graph wholesale); an empty mask
// or any other path is InvalidArgument. The merged workflow must still carry a
// name, a description and at least one tag.
func (s *WorkflowServer) UpdateWorkflow(
	ctx context.Context,
	req *connect.Request[metarrv1.UpdateWorkflowRequest],
) (*connect.Response[metarrv1.Workflow], error) {
	patch := req.Msg.GetWorkflow()
	if patch == nil {
		return nil, connectError(http.StatusBadRequest, errors.New("workflow is required"))
	}
	workflowID, err := parseRecordID(patch.GetId())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	for _, path := range req.Msg.GetUpdateMask().GetPaths() {
		if !workflowUpdatePaths[path] {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				errors.New("update_mask may name only name, description, tags or graph"))
		}
	}

	current, err := s.Store.GetLatest(ctx, workflowID)
	if err != nil {
		return nil, workflowLookupError(err, workflowID, s.Logger)
	}

	merged, err := workflowToProto(current)
	if err != nil {
		s.Logger.Error("failed to encode workflow", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to encode workflow"))
	}
	if maskErr := applyUpdateMask(merged, patch, req.Msg.GetUpdateMask()); maskErr != nil {
		return nil, aipConnectError(maskErr)
	}
	if err := validateWorkflowContent(merged); err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	stored, err := workflowFromProto(merged)
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}
	saved, err := s.Store.Save(ctx, workflowID, stored)
	if err != nil {
		s.Logger.Error("failed to save workflow", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to save workflow"))
	}
	return s.encodeWorkflow(saved)
}

// DeleteWorkflow hard-removes every version of the workflow (docs/adr/0013).
func (s *WorkflowServer) DeleteWorkflow(
	ctx context.Context,
	req *connect.Request[metarrv1.DeleteWorkflowRequest],
) (*connect.Response[emptypb.Empty], error) {
	workflowID, err := parseRecordID(req.Msg.GetId())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	if err := s.Store.DeleteAllVersions(ctx, workflowID); err != nil {
		if errors.Is(err, versioned.ErrNotFound) {
			return nil, connectError(http.StatusNotFound, errors.New("no workflow with that id"))
		}
		s.Logger.Error("failed to delete workflow", "workflow_id", workflowID.Hex(), "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to delete workflow"))
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (s *WorkflowServer) GetWorkflowVersion(
	ctx context.Context,
	req *connect.Request[metarrv1.GetWorkflowVersionRequest],
) (*connect.Response[metarrv1.Workflow], error) {
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
	return s.encodeWorkflow(workflow)
}

func (s *WorkflowServer) ListWorkflowVersions(
	ctx context.Context,
	req *connect.Request[metarrv1.ListWorkflowVersionsRequest],
) (*connect.Response[metarrv1.ListWorkflowVersionsResponse], error) {
	workflowID, err := parseRecordID(req.Msg.GetId())
	if err != nil {
		return nil, connectError(http.StatusBadRequest, err)
	}

	versions, err := s.Store.ListVersions(ctx, workflowID)
	if err != nil {
		s.Logger.Error("failed to list workflow versions", "workflow_id", workflowID.Hex(), "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to list workflow versions"))
	}

	protoVersions, err := s.encodeWorkflows(versions)
	if err != nil {
		return nil, err
	}

	page, nextToken, err := paginateSlice(protoVersions, req.Msg.GetPageSize(), req.Msg.GetPageToken())
	if err != nil {
		return nil, aipConnectError(err)
	}
	return connect.NewResponse(&metarrv1.ListWorkflowVersionsResponse{
		Workflows:     page,
		NextPageToken: nextToken,
	}), nil
}

// encodeWorkflow converts one stored workflow to its proto form, mapping an
// encoding failure to a 500 the same way every method did inline before.
func (s *WorkflowServer) encodeWorkflow(w mongostore.Workflow) (*connect.Response[metarrv1.Workflow], error) {
	proto, err := workflowToProto(w)
	if err != nil {
		s.Logger.Error("failed to encode workflow", "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to encode workflow"))
	}
	return connect.NewResponse(proto), nil
}

func (s *WorkflowServer) encodeWorkflows(workflows []mongostore.Workflow) ([]*metarrv1.Workflow, error) {
	out := make([]*metarrv1.Workflow, 0, len(workflows))
	for _, w := range workflows {
		proto, err := workflowToProto(w)
		if err != nil {
			s.Logger.Error("failed to encode workflow", "error", err)
			return nil, connectError(http.StatusInternalServerError, errors.New("failed to encode workflow"))
		}
		out = append(out, proto)
	}
	return out, nil
}

// validateWorkflowContent carries the pre-AIP Upsert rules forward: a workflow
// needs a name, a description and at least one tag. CreateWorkflow runs it on
// the request; UpdateWorkflow runs it on the merged result so a mask cannot
// strip a workflow below the minimum.
func validateWorkflowContent(w *metarrv1.Workflow) error {
	if w.GetName() == "" || w.GetDescription() == "" || len(w.GetTags()) == 0 {
		return errors.New("name, description and at least one tag are required")
	}
	return nil
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
		Id:          w.DocumentID.Hex(),
		Version:     int32(w.Version),
		CreatedAt:   timestamppb.New(w.CreatedAt),
		Name:        w.Name,
		Description: w.Description,
		Tags:        w.Tags,
		Graph:       graph,
	}, nil
}

// workflowFromProto turns a Workflow message (a Create body, or an
// UpdateWorkflow merged result) into the loose document mongostore persists.
// Only the content fields are read — id / version / created_at are the store's
// to assign.
func workflowFromProto(w *metarrv1.Workflow) (mongostore.Workflow, error) {
	stored, err := graphFromProto(w.GetGraph())
	if err != nil {
		return mongostore.Workflow{}, errors.New("malformed graph")
	}
	return mongostore.Workflow{
		Name:          w.GetName(),
		Description:   w.GetDescription(),
		Tags:          w.GetTags(),
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
