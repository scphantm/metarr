/*
 * These mirror the Go structs field-for-field, taken from their JSON tags in
 * internal/appconfig and internal/handlers. Where a Go field is a pointer with
 * `omitempty` — a partial-update request — the property here is optional, and
 * carries the same meaning: absent means "leave alone", which is not the same
 * as an empty string.
 */

import type { GraphEdge, GraphNode } from '../pages/workflows/catalogTypes'

// AdminUser is the single administrative account. The password salt and hash
// are redacted by GetConfig before they reach a client, so they are absent
// here by design rather than by omission.
export type AdminUser = {
  username: string
  email: string
}

export type APIKeyEntry = {
  name: string
  api_key: string
}

export type APIKeysConfig = {
  admin: APIKeyEntry[]
  user: APIKeyEntry[]
  webhook: APIKeyEntry[]
  read_only: APIKeyEntry[]
}

export type APIKeyGroup = keyof APIKeysConfig

export const apiKeyGroups: { key: APIKeyGroup; label: string; hint: string }[] =
  [
    { key: 'admin', label: 'Admin', hint: 'Full access to every endpoint' },
    { key: 'user', label: 'User', hint: 'Tasks and library reads' },
    { key: 'webhook', label: 'Webhook', hint: 'For inbound automation' },
    { key: 'read_only', label: 'Read only', hint: 'Library reads only' },
  ]

export type RootDirMapping = {
  sonarr_path: string
  local_path: string
}

// StorageConfig controls retention: "cache" expires after ttl, "versioned"
// keeps up to max_count revisions. Only the field belonging to the active mode
// is meaningful.
export type StorageConfig = {
  mode: string
  ttl?: string
  max_count?: number
}

export const storageModes = ['cache', 'versioned'] as const

export type SonarrInstance = {
  instance_name: string
  instance_slug: string
  sonarr_url: string
  sonarr_api_key: string
  root_dir_map: RootDirMapping[]
  storage: StorageConfig
}

export type InterfacesConfig = {
  sonarr: SonarrInstance[]
}

// DirectoryType is the closed vocabulary from mediascan.ParseDirectoryType.
export const directoryTypes = ['movie', 'tv', 'music_video'] as const
export type DirectoryType = (typeof directoryTypes)[number]

export type ScanDirectory = {
  scanner_slug: string
  scan_type: string
  directory: string
}

// SidecarCategory is closed on the Go side (mediascan.ParseSidecarCategory), so
// the editor offers exactly these and nothing else.
export const sidecarCategories = [
  'image',
  'video_extra',
  'subtitle',
  'metadata',
  'audio',
  'disc_structure',
  'trickplay',
  'unknown',
] as const
export type SidecarCategory = (typeof sidecarCategories)[number]

// SidecarTypeDefinition is one row of the classification table.
//
// `order` is the evaluation sequence and zero means disabled — the entry stays
// in the table, still editable, but is never evaluated. It is changed only
// through the dedicated ordering endpoint, never by editing a row, because
// uniqueness is a property of the whole table.
export type SidecarTypeDefinition = {
  id: string
  type: string
  category: string
  order: number
  patterns: string[]
  extensions: string[]
}

export type DirectoryScannerConfig = {
  parallel_count: number
  scan_directories: ScanDirectory[]
  sidecar_types: SidecarTypeDefinition[]
}

export type Config = {
  api_keys: APIKeysConfig
  admin: AdminUser
  interfaces: InterfacesConfig
  directory_scanner: DirectoryScannerConfig
}

// UpdateAdminRequest sends only what changed: an omitted field is left alone,
// and an explicitly empty one is rejected rather than silently clearing.
export type UpdateAdminRequest = {
  username?: string
  email?: string
  password?: string
}

export type UpdateDirectoryScannerRequest = {
  parallel_count?: number
}

// ReorderSidecarTypesRequest maps every sidecar type id to its order, covering
// the whole table in one transaction.
export type ReorderSidecarTypesRequest = Record<string, number>

// AcceptedResponse is what every mutation returns. The status is 202 and the
// write has only been queued at that point — see the note in api/client.ts.
export type AcceptedResponse = {
  status: string
  event: string
  correlation_id: string
}

export type LoginRequest = {
  username: string
  password: string
}

export type LoginResponse = {
  api_key: string
  expires_in_seconds: number
}

export type HeartbeatResponse = {
  time: string
  correlation_id: string
}

/*
 * Redis statistics, streamed over the stats.redis topic and also readable at
 * GET /api/stats/redis.
 *
 * The two collections here are not two flavours of the same thing. Streams are
 * durable — messages sit on them until acknowledged — so their depth and
 * pending counts are real numbers. Pub/Sub holds nothing at all, which is why
 * PubSubChannelStat has a subscriber count and no depth: there is none to
 * report.
 */

export type RedisServerInfo = {
  version: string
  uptime_seconds: number
  connected_clients: number
  used_memory: number
  used_memory_human: string
  ops_per_second: number
  total_keys: number
}

export type RedisConsumerStat = {
  name: string
  pending: number
  idle_seconds: number
}

export type RedisGroupStat = {
  name: string
  consumers: number
  pending: number
  lag: number
  last_delivered_id: string
  consumer_detail: RedisConsumerStat[]
}

export type RedisStreamStat = {
  stream: string
  event_name: string
  length: number
  // Streams are created lazily, when a listener first subscribes, so a length
  // of zero on its own cannot tell "empty" from "never created".
  exists: boolean
  groups: RedisGroupStat[]
  error?: string
}

export type PubSubChannelStat = {
  channel: string
  subscribers: number
  // false for the per-correlation-id reply channels, which exist only while a
  // request is in flight.
  known: boolean
}

export type RedisStats = {
  collected_at: string
  server: RedisServerInfo
  streams: RedisStreamStat[]
  pubsub: PubSubChannelStat[]
}

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
 * The server's own level, plus a per-agent level set on each AgentView above.
 * sink/endpoint/stream are informational only — they describe what Fluent Bit
 * is currently configured to ship to, and drive the "Open in OpenObserve"
 * link, but changing them here does not reconfigure the pipeline. Actually
 * repointing it at a different vendor is a Fluent Bit config change.
 */

export const logLevels = ['info', 'debug'] as const
export type LogLevel = (typeof logLevels)[number]

export type LoggingConfig = {
  server_level: string
  sink: string
  endpoint: string
  stream: string
}

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
