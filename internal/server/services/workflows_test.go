package services

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
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

	msg := &metarrv1.Workflow{
		Name: "keeps unknown things", Description: "d", Tags: []string{"t"}, Graph: original,
	}

	// Store: the message becomes the loose document mongostore persists.
	stored, err := workflowFromProto(msg)
	if err != nil {
		t.Fatalf("workflowFromProto: %v", err)
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

// TestWorkflowCreateAcceptsAnEmptyGraph pins that the retype did not make a
// blank canvas un-saveable — people save half-built flows all the time.
func TestWorkflowCreateAcceptsAnEmptyGraph(t *testing.T) {
	stored, err := workflowFromProto(&metarrv1.Workflow{
		Name: "n", Description: "d", Tags: []string{"t"},
		Graph: &metarrv1.WorkflowGraph{SchemaVersion: 1},
	})
	if err != nil {
		t.Fatalf("workflowFromProto on an empty graph: %v", err)
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

func createWorkflowReq(name string) *connect.Request[metarrv1.CreateWorkflowRequest] {
	return connect.NewRequest(&metarrv1.CreateWorkflowRequest{
		Workflow: &metarrv1.Workflow{
			Name:        name,
			Description: "d",
			Tags:        []string{"t"},
			Graph:       &metarrv1.WorkflowGraph{SchemaVersion: 1},
		},
	})
}

func mustCreateWorkflow(t *testing.T, srv *WorkflowServer, name string) *metarrv1.Workflow {
	t.Helper()
	res, err := srv.CreateWorkflow(context.Background(), createWorkflowReq(name))
	if err != nil {
		t.Fatalf("CreateWorkflow(%q): %v", name, err)
	}
	return res.Msg
}

// TestWorkflowCreateGetList drives the standard reads against the fake store:
// create mints an id at version 1, and Get / List reflect what was created.
func TestWorkflowCreateGetList(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)
	ctx := context.Background()

	created := mustCreateWorkflow(t, srv, "first")
	if created.GetId() == "" {
		t.Fatal("created workflow has no id")
	}
	if created.GetVersion() != 1 {
		t.Fatalf("created version = %d, want 1", created.GetVersion())
	}

	got, err := srv.GetWorkflow(ctx, connect.NewRequest(&metarrv1.GetWorkflowRequest{Id: created.GetId()}))
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if got.Msg.GetName() != "first" {
		t.Fatalf("GetWorkflow name = %q, want %q", got.Msg.GetName(), "first")
	}

	list, err := srv.ListWorkflows(ctx, connect.NewRequest(&metarrv1.ListWorkflowsRequest{}))
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if n := len(list.Msg.GetWorkflows()); n != 1 {
		t.Fatalf("ListWorkflows returned %d, want 1", n)
	}
}

func TestWorkflowCreateRejectsMissingContent(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)

	_, err := srv.CreateWorkflow(context.Background(), connect.NewRequest(&metarrv1.CreateWorkflowRequest{
		Workflow: &metarrv1.Workflow{Name: "n", Description: "d", Graph: &metarrv1.WorkflowGraph{SchemaVersion: 1}},
	}))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("CreateWorkflow without tags code = %v, want %v", got, connect.CodeInvalidArgument)
	}
}

func TestWorkflowCreateRejectsClientSuppliedID(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)

	req := createWorkflowReq("first")
	req.Msg.Workflow.Id = bson.NewObjectID().Hex()
	_, err := srv.CreateWorkflow(context.Background(), req)
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("CreateWorkflow with an id code = %v, want %v", got, connect.CodeInvalidArgument)
	}
}

// TestWorkflowUpdateAppendsVersion pins that UpdateWorkflow appends an
// immutable version and leaves the earlier one fetchable by version.
func TestWorkflowUpdateAppendsVersion(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)
	ctx := context.Background()

	created := mustCreateWorkflow(t, srv, "first")
	id := created.GetId()

	updated, err := srv.UpdateWorkflow(ctx, connect.NewRequest(&metarrv1.UpdateWorkflowRequest{
		Workflow:   &metarrv1.Workflow{Id: id, Name: "second"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	}))
	if err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	if updated.Msg.GetVersion() != 2 {
		t.Fatalf("updated version = %d, want 2", updated.Msg.GetVersion())
	}
	if updated.Msg.GetName() != "second" {
		t.Fatalf("updated name = %q, want %q", updated.Msg.GetName(), "second")
	}
	// The mask named only name, so description and tags carry over.
	if updated.Msg.GetDescription() != "d" || len(updated.Msg.GetTags()) != 1 {
		t.Fatalf("unmasked fields not preserved: %+v", updated.Msg)
	}

	v1, err := srv.GetWorkflowVersion(ctx, connect.NewRequest(&metarrv1.GetWorkflowVersionRequest{Id: id, Version: 1}))
	if err != nil {
		t.Fatalf("GetWorkflowVersion 1: %v", err)
	}
	if v1.Msg.GetName() != "first" {
		t.Fatalf("version 1 name = %q, want %q", v1.Msg.GetName(), "first")
	}

	versions, err := srv.ListWorkflowVersions(ctx, connect.NewRequest(&metarrv1.ListWorkflowVersionsRequest{Id: id}))
	if err != nil {
		t.Fatalf("ListWorkflowVersions: %v", err)
	}
	if n := len(versions.Msg.GetWorkflows()); n != 2 {
		t.Fatalf("ListWorkflowVersions returned %d, want 2", n)
	}
}

func TestWorkflowUpdateMaskErrors(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)
	ctx := context.Background()
	id := mustCreateWorkflow(t, srv, "first").GetId()

	cases := map[string]*fieldmaskpb.FieldMask{
		"empty mask":      {},
		"unknown path":    {Paths: []string{"schema_version"}},
		"graph sub-path":  {Paths: []string{"graph.nodes"}},
		"nil mask":        nil,
		"created_at path": {Paths: []string{"created_at"}},
		"version path":    {Paths: []string{"version"}},
		"id path in mask": {Paths: []string{"id"}},
	}
	for name, mask := range cases {
		_, err := srv.UpdateWorkflow(ctx, connect.NewRequest(&metarrv1.UpdateWorkflowRequest{
			Workflow:   &metarrv1.Workflow{Id: id, Name: "x"},
			UpdateMask: mask,
		}))
		if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
			t.Fatalf("%s: code = %v, want %v", name, got, connect.CodeInvalidArgument)
		}
	}
}

// TestWorkflowUpdateRejectsEmptiedTags pins that the merged result is
// re-validated: a mask that clears tags cannot land.
func TestWorkflowUpdateRejectsEmptiedTags(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)
	id := mustCreateWorkflow(t, srv, "first").GetId()

	_, err := srv.UpdateWorkflow(context.Background(), connect.NewRequest(&metarrv1.UpdateWorkflowRequest{
		Workflow:   &metarrv1.Workflow{Id: id, Tags: nil},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tags"}},
	}))
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("update emptying tags code = %v, want %v", got, connect.CodeInvalidArgument)
	}
}

func TestWorkflowUpdateUnknownIDIsNotFound(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)

	_, err := srv.UpdateWorkflow(context.Background(), connect.NewRequest(&metarrv1.UpdateWorkflowRequest{
		Workflow:   &metarrv1.Workflow{Id: bson.NewObjectID().Hex(), Name: "x"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("UpdateWorkflow unknown id code = %v, want %v", got, connect.CodeNotFound)
	}
}

// TestWorkflowDeleteRemovesEveryVersion pins the surprising call in an
// append-only store: delete drops all versions and a later Get is NotFound.
func TestWorkflowDeleteRemovesEveryVersion(t *testing.T) {
	srv, store := newTestWorkflowServer(t)
	ctx := context.Background()
	id := mustCreateWorkflow(t, srv, "first").GetId()

	if _, err := srv.UpdateWorkflow(ctx, connect.NewRequest(&metarrv1.UpdateWorkflowRequest{
		Workflow:   &metarrv1.Workflow{Id: id, Name: "second"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	})); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}

	if _, err := srv.DeleteWorkflow(ctx, connect.NewRequest(&metarrv1.DeleteWorkflowRequest{Id: id})); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}

	docID, _ := bson.ObjectIDFromHex(id)
	if remaining, _ := store.ListVersions(ctx, docID); len(remaining) != 0 {
		t.Fatalf("delete left %d versions behind", len(remaining))
	}

	_, err := srv.GetWorkflow(ctx, connect.NewRequest(&metarrv1.GetWorkflowRequest{Id: id}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("GetWorkflow after delete code = %v, want %v", got, connect.CodeNotFound)
	}

	_, err = srv.DeleteWorkflow(ctx, connect.NewRequest(&metarrv1.DeleteWorkflowRequest{Id: id}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("second DeleteWorkflow code = %v, want %v", got, connect.CodeNotFound)
	}
}

// TestWorkflowListPaginates pins page_size / page_token threading through the
// seam and the empty next_page_token that ends the list.
func TestWorkflowListPaginates(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)
	ctx := context.Background()

	for _, name := range []string{"a", "b", "c"} {
		mustCreateWorkflow(t, srv, name)
	}

	first, err := srv.ListWorkflows(ctx, connect.NewRequest(&metarrv1.ListWorkflowsRequest{PageSize: 2}))
	if err != nil {
		t.Fatalf("ListWorkflows page 1: %v", err)
	}
	if n := len(first.Msg.GetWorkflows()); n != 2 {
		t.Fatalf("page 1 returned %d, want 2", n)
	}
	if first.Msg.GetNextPageToken() == "" {
		t.Fatal("page 1 has no next_page_token")
	}

	second, err := srv.ListWorkflows(ctx, connect.NewRequest(&metarrv1.ListWorkflowsRequest{
		PageSize:  2,
		PageToken: first.Msg.GetNextPageToken(),
	}))
	if err != nil {
		t.Fatalf("ListWorkflows page 2: %v", err)
	}
	if n := len(second.Msg.GetWorkflows()); n != 1 {
		t.Fatalf("page 2 returned %d, want 1", n)
	}
	if second.Msg.GetNextPageToken() != "" {
		t.Fatal("page 2 should be the last page")
	}
}

func TestWorkflowListFilterIsUnimplemented(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)

	_, err := srv.ListWorkflows(context.Background(), connect.NewRequest(&metarrv1.ListWorkflowsRequest{
		Filter: `name = "x"`,
	}))
	if got := connect.CodeOf(err); got != connect.CodeUnimplemented {
		t.Fatalf("ListWorkflows filter code = %v, want %v", got, connect.CodeUnimplemented)
	}
}

func TestWorkflowGetUnknownIsNotFound(t *testing.T) {
	srv, _ := newTestWorkflowServer(t)

	_, err := srv.GetWorkflow(context.Background(), connect.NewRequest(&metarrv1.GetWorkflowRequest{
		Id: bson.NewObjectID().Hex(),
	}))
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("GetWorkflow unknown id code = %v, want %v", got, connect.CodeNotFound)
	}
}
