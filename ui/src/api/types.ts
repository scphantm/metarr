/*
 * The still-hand-written half of the model types. Each family below is
 * retired to its generated metarr.v1 message as its migration slice lands
 * (see docs/adr/0005); the config families — application config, directory
 * scanner, Sonarr, logging config — have already gone. Closed vocabularies
 * the config screens use live in ./vocab.
 */

import type { GraphEdge, GraphNode } from '../pages/workflows/catalogTypes'

// ReorderSidecarTypesRequest maps every sidecar type id to its order, covering
// the whole table in one transaction. Not a model — a map keyed by id.
export type ReorderSidecarTypesRequest = Record<string, number>

// AcceptedResponse is what every mutation returns. The status is 202 and the
// write has only been queued at that point — see the note in api/client.ts.
export type AcceptedResponse = {
  status: string
  event: string
  correlation_id: string
}

// Redis statistics are the generated metarr.v1.RedisSnapshot now — see
// ../gen/metarr/v1/stats_pb.

// Agents are the generated metarr.v1 messages now — AgentView / AgentIdentity /
// AgentTelemetry / GPUTelemetry / AgentMappingView / AgentConfig from
// ../gen/metarr/v1/agents_pb.

/*
 * Logging.
 *
 * The logging config itself is the generated metarr.v1.LoggingConfig now;
 * only the tail entry stays hand-written until the log-record slice lands.
 */

export type LogTailEntry = {
  time: string
  level: string
  message: string
  source: string
  attrs?: Record<string, unknown>
}

/*
 * Workflows.
 *
 * A Workflow is one version of a versioned document (see
 * internal/server/mongostore/versioned): document_id groups every version of
 * the same logical workflow, id is this specific version's own identity, and
 * every save produces a brand new version rather than overwriting one in
 * place. nodes/edges/viewport are the canonical graph shape from
 * internal/shared/workflow/graph.go (see ../pages/workflows/catalogTypes.ts
 * for the field-for-field mirror) — schema_version says which shape they're
 * in; a document without a matching one predates the control/data-edge
 * redesign and is opened read-only rather than guessed at.
 */

export type Workflow = {
  id: string
  document_id: string
  version: number
  created_at: string
  name: string
  description: string
  tags: string[]
  schema_version: number
  nodes: GraphNode[]
  edges: GraphEdge[]
  viewport: Record<string, unknown>
}

export type WorkflowListResponse = {
  workflows: Workflow[]
  next_cursor?: string
  has_more: boolean
}

export type UpsertWorkflowRequest = {
  document_id?: string
  name: string
  description: string
  tags: string[]
  schema_version: number
  nodes: GraphNode[]
  edges: GraphEdge[]
  viewport: Record<string, unknown>
}
