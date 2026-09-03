package services

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/mongostore"
)

func mustStruct(t *testing.T, fields map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(fields)
	if err != nil {
		t.Fatalf("building struct: %v", err)
	}
	return s
}

// TestWorkflowGraphSurvivesStoreAndLoad is the single most important test in
// the model-generation work: the guard against silently destroying a user's
// workflow.
//
// A node whose type this build does not recognise, and settings it does not
// recognise, must round-trip through the workflow service's store-and-load
// conversion unchanged. The graph carries its open content as structured
// values (settings and extra) precisely so a typed schema cannot drop it —
// the design's passthrough promise, asserted here as external behaviour at
// the service seam rather than left as a comment warning someone not to
// break it.
func TestWorkflowGraphSurvivesStoreAndLoad(t *testing.T) {
	original := &metarrv1.WorkflowGraph{
		SchemaVersion: 1,
		Nodes: []*metarrv1.WorkflowGraphNode{
			{
				Id:        "n1",
				Type:      "vendor/node-this-build-has-never-heard-of",
				CatalogId: "vendor/node@9.9.9",
				Position:  &metarrv1.WorkflowGraphPosition{X: 12.5, Y: -40},
				Settings: mustStruct(t, map[string]any{
					"knownLooking":  "value",
					"unrecognised":  42.0,
					"deeplyUnknown": map[string]any{"keep": true, "list": []any{1.0, 2.0}},
				}),
				Promoted: []string{"knownLooking"},
				Label:    "keep my label",
				Extra: mustStruct(t, map[string]any{
					"shapeColor":  "violet",
					"borderColor": "cyan",
					"futureField": "from a newer build",
				}),
			},
			{
				Id:       "n2",
				Type:     "core/end",
				Position: &metarrv1.WorkflowGraphPosition{X: 0, Y: 0},
			},
		},
		Edges: []*metarrv1.WorkflowGraphEdge{
			{
				Id:   "e1",
				Kind: metarrv1.WorkflowEdgeKind_WORKFLOW_EDGE_KIND_CONTROL,
				From: &metarrv1.WorkflowEndpoint{Node: "n1", Port: "next"},
				To:   &metarrv1.WorkflowEndpoint{Node: "n2", Port: "in"},
			},
			{
				Id:        "e2",
				Kind:      metarrv1.WorkflowEdgeKind_WORKFLOW_EDGE_KIND_DATA,
				From:      &metarrv1.WorkflowEndpoint{Node: "n1", Port: "out"},
				To:        &metarrv1.WorkflowEndpoint{Node: "n2", Port: "value"},
				Transform: "parentDir",
				Settings:  mustStruct(t, map[string]any{"recursive": true}),
			},
		},
		Viewport: mustStruct(t, map[string]any{"x": 1.0, "y": 2.0, "zoom": 1.5}),
	}

	req := &metarrv1.WorkflowServiceUpsertRequest{
		Name: "keeps unknown things", Description: "d", Tags: []string{"t"}, Graph: original,
	}

	// Store: the request becomes the loose document mongostore persists.
	stored, err := workflowFromUpsertProto(req)
	if err != nil {
		t.Fatalf("workflowFromUpsertProto: %v", err)
	}

	// Simulate the Mongo round trip the versioned store performs — BSON
	// encode and decode of the whole document — so a type coercion in the
	// driver is caught here rather than in production.
	raw, err := bson.Marshal(stored)
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}
	var reloaded mongostore.Workflow
	if err := bson.Unmarshal(raw, &reloaded); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}

	// Load: the stored document becomes the graph message again.
	loaded, err := graphToProto(reloaded)
	if err != nil {
		t.Fatalf("graphToProto: %v", err)
	}

	if !proto.Equal(original, loaded) {
		t.Fatalf("graph did not survive store-and-load:\n before: %v\n after:  %v", original, loaded)
	}
}

// TestWorkflowUpsertAcceptsAnEmptyGraph pins that the retype did not make a
// blank canvas un-saveable — people save half-built flows all the time.
func TestWorkflowUpsertAcceptsAnEmptyGraph(t *testing.T) {
	// An empty graph is legitimate — people save blank canvases — so the
	// conversion must not reject it.
	stored, err := workflowFromUpsertProto(&metarrv1.WorkflowServiceUpsertRequest{
		Name: "n", Description: "d", Tags: []string{"t"},
		Graph: &metarrv1.WorkflowGraph{SchemaVersion: 1},
	})
	if err != nil {
		t.Fatalf("workflowFromUpsertProto on an empty graph: %v", err)
	}
	if stored.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", stored.SchemaVersion)
	}
}

// newTestWorkflowServer builds a WorkflowServer backed by an in-memory
// WorkflowStore — the fake-backed, handler-level seam the prefactor (#111)
// exists to make possible.
func newTestWorkflowServer(t *testing.T) (*WorkflowServer, *fakeWorkflowStore) {
	t.Helper()
	store := newFakeWorkflowStore()
	return &WorkflowServer{
		Handlers: &handlers.Handlers{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
		Store:    store,
	}, store
}

func upsertReq(name, documentID string) *connect.Request[metarrv1.WorkflowServiceUpsertRequest] {
	return connect.NewRequest(&metarrv1.WorkflowServiceUpsertRequest{
		DocumentId:  documentID,
		Name:        name,
		Description: "d",
		Tags:        []string{"t"},
		Graph:       &metarrv1.WorkflowGraph{SchemaVersion: 1},
	})
}

// TestWorkflowServerSeamRoundTrips drives the service end to end against the
// fake store: create mints a document id at version 1, a second save on that
// id appends version 2, and earlier versions stay fetchable.
func TestWorkflowServerSeamRoundTrips(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)
	ctx := context.Background()

	created, err := srv.Upsert(ctx, upsertReq("first", ""))
	if err != nil {
		t.Fatalf("Upsert create: %v", err)
	}
	id := created.Msg.GetWorkflow().GetDocumentId()
	if id == "" {
		t.Fatal("created workflow has no document id")
	}
	if got := created.Msg.GetWorkflow().GetVersion(); got != 1 {
		t.Fatalf("created version = %d, want 1", got)
	}

	got, err := srv.Get(ctx, connect.NewRequest(&metarrv1.WorkflowServiceGetRequest{Id: id}))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Msg.GetWorkflow().GetName() != "first" {
		t.Fatalf("Get name = %q, want %q", got.Msg.GetWorkflow().GetName(), "first")
	}

	updated, err := srv.Upsert(ctx, upsertReq("second", id))
	if err != nil {
		t.Fatalf("Upsert update: %v", err)
	}
	if got := updated.Msg.GetWorkflow().GetVersion(); got != 2 {
		t.Fatalf("updated version = %d, want 2", got)
	}

	v1, err := srv.GetVersion(ctx, connect.NewRequest(&metarrv1.WorkflowServiceGetVersionRequest{Id: id, Version: 1}))
	if err != nil {
		t.Fatalf("GetVersion 1: %v", err)
	}
	if v1.Msg.GetWorkflow().GetName() != "first" {
		t.Fatalf("version 1 name = %q, want %q", v1.Msg.GetWorkflow().GetName(), "first")
	}

	versions, err := srv.ListVersions(ctx, connect.NewRequest(&metarrv1.WorkflowServiceListVersionsRequest{Id: id}))
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if n := len(versions.Msg.GetVersions()); n != 2 {
		t.Fatalf("ListVersions returned %d, want 2", n)
	}

	list, err := srv.List(ctx, connect.NewRequest(&metarrv1.WorkflowServiceListRequest{}))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if n := len(list.Msg.GetWorkflows()); n != 1 {
		t.Fatalf("List returned %d latest workflows, want 1", n)
	}
}

// TestWorkflowServerListPaginatesThroughSeam pins that limit/cursor
// pagination still threads through the interface unchanged.
func TestWorkflowServerListPaginatesThroughSeam(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)
	ctx := context.Background()

	for _, name := range []string{"a", "b", "c"} {
		if _, err := srv.Upsert(ctx, upsertReq(name, "")); err != nil {
			t.Fatalf("Upsert %s: %v", name, err)
		}
	}

	first, err := srv.List(ctx, connect.NewRequest(&metarrv1.WorkflowServiceListRequest{Limit: 2}))
	if err != nil {
		t.Fatalf("List page 1: %v", err)
	}
	if !first.Msg.GetHasMore() || first.Msg.GetNextCursor() == "" {
		t.Fatalf("page 1 hasMore=%v nextCursor=%q, want more", first.Msg.GetHasMore(), first.Msg.GetNextCursor())
	}
	if n := len(first.Msg.GetWorkflows()); n != 2 {
		t.Fatalf("page 1 returned %d, want 2", n)
	}

	second, err := srv.List(ctx, connect.NewRequest(&metarrv1.WorkflowServiceListRequest{
		Limit:  2,
		Cursor: first.Msg.GetNextCursor(),
	}))
	if err != nil {
		t.Fatalf("List page 2: %v", err)
	}
	if n := len(second.Msg.GetWorkflows()); n != 1 {
		t.Fatalf("page 2 returned %d, want 1", n)
	}
	if second.Msg.GetHasMore() {
		t.Fatal("page 2 should be the last page")
	}
}

// TestWorkflowServerGetUnknownIsNotFound pins the error mapping through the
// seam: a versioned.ErrNotFound from the store becomes CodeNotFound.
func TestWorkflowServerGetUnknownIsNotFound(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)

	_, err := srv.Get(context.Background(), connect.NewRequest(&metarrv1.WorkflowServiceGetRequest{
		Id: bson.NewObjectID().Hex(),
	}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("Get unknown id code = %v, want %v", got, connect.CodeNotFound)
	}
}
