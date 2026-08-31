// Package run holds the workflow run audit log's model types. Every type here
// is an alias to the generated metarr.v1 message that defines it: proto is
// the single definition for a model that crosses a language boundary, and the
// run and node-execution records are stored Mongo documents. See
// docs/adr/0005 and the workflow engine design's "Run state" section.
//
// Nothing reads these yet — the execution engine is not built (design build
// order step 5). They are defined now so that when it is, the audit log is
// already a generated model and no hand-written struct is ever written for
// it. That is also why this package carries no logic: it is the model's home,
// not its store.
package run

import metarrv1 "Metarr/internal/genproto/metarr/v1"

// The run audit log model.
//
//   - Run is one document in workflow_runs.
//   - NodeExecution is one document in workflow_node_executions, keyed by
//     (run_id, node_id, frame, attempt).
//
// Neither collection is versioned: a run is mutated hundreds of times, not
// saved as immutable versions.
type (
	Run           = metarrv1.WorkflowRun
	NodeExecution = metarrv1.WorkflowNodeExecution
	Trigger       = metarrv1.WorkflowRunTrigger
	Error         = metarrv1.WorkflowRunError
	Counters      = metarrv1.WorkflowRunCounters
)
