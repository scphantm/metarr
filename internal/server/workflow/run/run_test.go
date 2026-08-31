package run

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestRunRecordRoundTripsThroughProtoJSON is the regression net for the run
// audit log's persistence shape. The engine will store these documents with
// protojson (the same way the config store does — see docs/adr/0005), so a
// fully populated record has to survive a marshal/unmarshal unchanged. A
// future edit that swaps one of these fields for a shape protojson cannot
// round-trip losslessly fails here rather than silently dropping part of an
// audit record.
func TestRunRecordRoundTripsThroughProtoJSON(t *testing.T) {
	payload, err := structpb.NewStruct(map[string]any{"libraryId": "lib-7", "depth": 3})
	if err != nil {
		t.Fatalf("building payload struct: %v", err)
	}
	graph, err := structpb.NewStruct(map[string]any{"nodes": []any{}, "edges": []any{}})
	if err != nil {
		t.Fatalf("building graph struct: %v", err)
	}

	original := &Run{
		Id:                 "run-1",
		WorkflowDocumentId: "wf-9",
		WorkflowVersion:    4,
		Graph:              graph,
		CatalogSnapshot:    graph,
		Trigger:            &Trigger{Kind: "manual", By: "scphantm", Payload: payload},
		Inputs:             payload,
		Mode:               "development",
		DryRun:             true,
		LogLevel:           "debug",
		WorkDir:            "workflows/run-1",
		Status:             "running",
		StartedAt:          timestamppb.New(timestamppb.Now().AsTime()),
		Error:              &Error{NodeId: "n3", Frame: "/n1#0", Code: "handler_failed", Message: "boom"},
		Counters:           &Counters{Executed: 12, Failed: 1, Skipped: 2, Cancelled: 0},
		EngineInstanceId:   "engine-a",
		LeaseExpiresAt:     timestamppb.New(timestamppb.Now().AsTime()),
		Breakpoints:        []string{"n5", "n8"},
	}

	encoded, err := protojson.Marshal(original)
	if err != nil {
		t.Fatalf("marshalling run: %v", err)
	}

	var decoded Run
	if err := protojson.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshalling run: %v", err)
	}

	if !proto.Equal(original, &decoded) {
		t.Fatalf("run did not survive protojson round trip:\n before: %v\n  after: %v", original, &decoded)
	}
}

// TestNodeExecutionRoundTripsThroughProtoJSON is the same guard for the
// per-execution record.
func TestNodeExecutionRoundTripsThroughProtoJSON(t *testing.T) {
	outputs, err := structpb.NewStruct(map[string]any{"path": "/media/show/s01e01.mkv", "sizeBytes": 1024})
	if err != nil {
		t.Fatalf("building outputs struct: %v", err)
	}

	original := &NodeExecution{
		Id:                    "exec-1",
		RunId:                 "run-1",
		NodeId:                "n3",
		Frame:                 "/n1#0",
		Attempt:               2,
		Status:                "succeeded",
		AgentSlug:             "nas-01",
		DispatchCorrelationId: "corr-abc",
		InputsResolved:        outputs,
		Outputs:               outputs,
		StartedAt:             timestamppb.New(timestamppb.Now().AsTime()),
		FinishedAt:            timestamppb.New(timestamppb.Now().AsTime()),
		DurationMs:            4200,
	}

	encoded, err := protojson.Marshal(original)
	if err != nil {
		t.Fatalf("marshalling execution: %v", err)
	}

	var decoded NodeExecution
	if err := protojson.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshalling execution: %v", err)
	}

	if !proto.Equal(original, &decoded) {
		t.Fatalf("execution did not survive protojson round trip:\n before: %v\n  after: %v", original, &decoded)
	}
}
