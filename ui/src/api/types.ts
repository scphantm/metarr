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

/*
 * Agents.
 *
 * An agent is a small binary deployed next to the media it scans. It connects
 * to Redis and nothing else, and carries only a slug and a Redis connection
 * locally — everything else it knows is published to it by the server.
 *
 * The two halves of an agent's state come from different places and mean
 * different things. `configured` is what an operator has said about it and
 * lives in the config document; `online` is whether it is currently refreshing
 * its presence key in Redis, which is volatile and stored nowhere else. A card
 * can be configured and offline (a machine that is switched off), or online and
 * unconfigured (a machine that has just appeared and needs setting up).
 */

export type AgentIdentity = {
  slug: string
  instance_id: string
  hostname: string
  ip: string
  uid: number
  username: string
  os: string
  arch: string
  version: string
  started: string
}

export type GPUTelemetry = {
  name: string
  utilization_percent: number
  memory_used_bytes: number
  memory_total_bytes: number
}

export type AgentTelemetry = {
  cpu_percent: number
  memory_used_bytes: number
  memory_total_bytes: number
  gpus?: GPUTelemetry[]
}

export type AgentMappingView = {
  scanner_slug: string
  scan_type: string
  server_path: string
  agent_path: string
}

export type AgentView = {
  slug: string
  display_name?: string
  online: boolean
  configured: boolean
  identity?: AgentIdentity
  telemetry?: AgentTelemetry
  reported_at?: string
  mappings: AgentMappingView[]
  log_level: string
}

// AgentDirectoryMapping is the write shape: only the two fields the operator
// actually sets. The server path and scan type are looked up from the scan
// directory, so sending them back would be a second source of truth.
export type AgentDirectoryMapping = {
  scanner_slug: string
  agent_path: string
}

export type AgentConfig = {
  slug: string
  display_name?: string
  mappings: AgentDirectoryMapping[]
}

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
