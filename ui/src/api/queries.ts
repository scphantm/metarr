import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
} from '@tanstack/react-query'

import {
  agentClient,
  configClient,
  directoryScannerClient,
  loggingClient,
  sonarrInterfaceClient,
  statsClient,
  workflowCatalogClient,
  workflowClient,
} from './clients'
import { mapAsync, registerStream, Stream, useStream, useStreamStatus } from './streams'
import type {
  AcceptedResponse,
  AgentConfig,
  AgentView,
  DirectoryScannerConfig,
  LoggingConfig,
  LogTailEntry,
  RedisStats,
  ReorderSidecarTypesRequest,
  ScanDirectory,
  SidecarTypeDefinition,
  UpdateDirectoryScannerRequest,
  UpsertWorkflowRequest,
  Workflow,
  WorkflowListResponse,
} from './types'
import type { MessageInitShape } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import type { AcceptedResponse as ConnectAcceptedResponse } from '../gen/metarr/v1/common_pb'
import { SonarrInstanceSchema } from '../gen/metarr/v1/sonarr_interfaces_pb'
import {
  ConfigServiceDeleteApiKeyRequestSchema,
  ConfigServiceUpdateAdminRequestSchema,
  ConfigServiceUpsertApiKeyRequestSchema,
} from '../gen/metarr/v1/config_pb'
import type { AgentView as ConnectAgentView } from '../gen/metarr/v1/agents_pb'
import type {
  ScanDirectory as ConnectScanDirectory,
  SidecarTypeDefinition as ConnectSidecarTypeDefinition,
} from '../gen/metarr/v1/directory_scanner_pb'
import type { Workflow as ConnectWorkflow } from '../gen/metarr/v1/workflows_pb'
import type { CatalogResponse, GraphEdge, GraphNode } from '../pages/workflows/catalogTypes'

export const queryKeys = {
  config: ['config'] as const,
  directoryScanner: ['config', 'directory-scanner'] as const,
  scanDirectories: ['config', 'scan-directories'] as const,
  sidecarTypes: ['config', 'sidecar-types'] as const,
  sonarr: ['config', 'interfaces', 'sonarr'] as const,
  // Deliberately outside the config tree: these are fed by a socket, and the
  // config-wide invalidations should not reach them.
  redisStats: ['stats', 'redis'] as const,
  agents: ['stats', 'agents'] as const,
  logging: ['config', 'logging'] as const,
  logTail: ['stats', 'log-tail'] as const,
  // Outside the config tree: workflows are a server-only, single-
  // collection concern with no config-mutation event behind them at all.
  workflows: ['workflows'] as const,
  workflow: (id: string) => ['workflows', id] as const,
  workflowVersions: (id: string) => ['workflows', id, 'versions'] as const,
  workflowCatalog: ['workflows', 'catalog'] as const,
}

/*
 * Reads.
 *
 * Everything below writes through a system_config_update event, so a mutation
 * settling does not mean the read is fresh yet. The mutations invalidate their
 * queries, and the sections that own them poll briefly while a save is
 * outstanding — see useConfirmationPoll.
 */

export function useConfig() {
  return useQuery({
    queryKey: queryKeys.config,
    queryFn: async () => (await configClient.get({})).config,
  })
}

// Neither of these reads streams over a socket, but useScanDirectories'
// output is also consumed by AgentConfigureForm (Agents domain, migrated in
// the previous step to the same REST-era shape) — so both stay mapped down
// to the snake_case shape from types.ts rather than the raw camelCase proto
// type, keeping every consumer's field names stable across this migration.
function connectScanDirectoryToLegacyShape(dir: ConnectScanDirectory): ScanDirectory {
  return { scanner_slug: dir.scannerSlug, scan_type: dir.scanType, directory: dir.directory }
}

function connectSidecarTypeToLegacyShape(def: ConnectSidecarTypeDefinition): SidecarTypeDefinition {
  return {
    id: def.id,
    type: def.type,
    category: def.category,
    order: def.order,
    patterns: def.patterns,
    extensions: def.extensions,
  }
}

export function useDirectoryScannerConfig() {
  return useQuery({
    queryKey: queryKeys.directoryScanner,
    queryFn: async () => {
      const config = (await directoryScannerClient.get({})).config
      return {
        parallel_count: config?.parallelCount ?? 0,
        scan_directories: (config?.scanDirectories ?? []).map(connectScanDirectoryToLegacyShape),
        sidecar_types: (config?.sidecarTypes ?? []).map(connectSidecarTypeToLegacyShape),
      } satisfies DirectoryScannerConfig
    },
  })
}

export function useScanDirectories() {
  return useQuery({
    queryKey: queryKeys.scanDirectories,
    queryFn: async () =>
      (await directoryScannerClient.listDirectories({})).directories.map(
        connectScanDirectoryToLegacyShape,
      ),
  })
}

export function useSidecarTypes() {
  return useQuery({
    queryKey: queryKeys.sidecarTypes,
    queryFn: async () =>
      (await directoryScannerClient.listSidecarTypes({})).types.map(
        connectSidecarTypeToLegacyShape,
      ),
  })
}

export function useSonarrInstances() {
  return useQuery({
    queryKey: queryKeys.sonarr,
    queryFn: async () => (await sonarrInterfaceClient.list({})).instances,
  })
}

// Redis statistics arrive over the socket rather than by refetching: useTopic
// writes each frame straight into this query's cache entry. The queryFn still
// runs once, so the page has something to paint before the socket is up and
// something to fall back on if it never connects.
// Agents stream over the socket for the same reason the Redis stats do: the
// telemetry is live and the presence half changes on its own, with no user
// action to hang a refetch off. The queryFn covers the first paint.
// agents.presence still streams the REST-era, snake_case AgentView shape
// (that migrates in the streaming step) — so List's response is mapped down
// to the exact same shape here, letting every socket frame and every refetch
// land in the same cache entry without a shape mismatch.
function connectAgentViewToLegacyShape(agent: ConnectAgentView): AgentView {
  return {
    slug: agent.slug,
    display_name: agent.displayName || undefined,
    online: agent.online,
    configured: agent.configured,
    identity: agent.identity
      ? {
          slug: agent.identity.slug,
          instance_id: agent.identity.instanceId,
          hostname: agent.identity.hostname,
          ip: agent.identity.ip,
          uid: agent.identity.uid,
          username: agent.identity.username,
          os: agent.identity.os,
          arch: agent.identity.arch,
          version: agent.identity.version,
          started: (agent.identity.started
            ? timestampDate(agent.identity.started)
            : new Date(0)
          ).toISOString(),
        }
      : undefined,
    telemetry: agent.telemetry
      ? {
          cpu_percent: agent.telemetry.cpuPercent,
          memory_used_bytes: Number(agent.telemetry.memoryUsedBytes),
          memory_total_bytes: Number(agent.telemetry.memoryTotalBytes),
          gpus: agent.telemetry.gpus.map((gpu) => ({
            name: gpu.name,
            utilization_percent: gpu.utilizationPercent,
            memory_used_bytes: Number(gpu.memoryUsedBytes),
            memory_total_bytes: Number(gpu.memoryTotalBytes),
          })),
        }
      : undefined,
    reported_at: agent.reportedAt ? timestampDate(agent.reportedAt).toISOString() : undefined,
    mappings: agent.mappings.map((mapping) => ({
      scanner_slug: mapping.scannerSlug,
      scan_type: mapping.scanType,
      server_path: mapping.serverPath,
      agent_path: mapping.agentPath,
    })),
    log_level: agent.logLevel,
  }
}

// One singleton per server-streaming RPC, refcounted across every component
// watching it — see streams.ts. Registered so resetStreams() (called on
// sign-out from AuthContext.clearSession) can close all three at once.
const agentsPresenceStream = registerStream(
  new Stream((signal) =>
    mapAsync(agentClient.streamPresence({}, { signal }), (response) =>
      response.agents.map(connectAgentViewToLegacyShape),
    ),
  ),
)

export function useAgentsPresenceStreamStatus() {
  return useStreamStatus(agentsPresenceStream)
}

export function useAgents() {
  useStream(agentsPresenceStream, queryKeys.agents)

  return useQuery({
    queryKey: queryKeys.agents,
    queryFn: async () =>
      (await agentClient.list({})).agents.map(connectAgentViewToLegacyShape),
    staleTime: Infinity,
  })
}

// Agents are upserted by slug, like every other config collection here. The
// call site keeps the same snake_case shape it always has (AgentConfig from
// types.ts) — only the transport underneath changed.
export function useUpsertAgent() {
  return useConfigMutation<AgentConfig, ConnectAcceptedResponse>(
    (body) =>
      agentClient.upsert({
        agent: {
          slug: body.slug,
          displayName: body.display_name ?? '',
          mappings: body.mappings.map((mapping) => ({
            scannerSlug: mapping.scanner_slug,
            agentPath: mapping.agent_path,
          })),
        },
      }),
    [queryKeys.config, queryKeys.agents],
  )
}

export function useDeleteAgent() {
  return useConfigMutation<string, ConnectAcceptedResponse>(
    (slug) => agentClient.delete({ slug }),
    [queryKeys.config, queryKeys.agents],
  )
}

// StatsService.Get/Stream both carry the exact same opaque JSON
// redisstats.Snapshot already produced over REST/WebSocket, so it decodes
// straight into the same RedisStats shape with no field-name translation.
const redisStatsStream = registerStream(
  new Stream((signal) =>
    mapAsync(
      statsClient.stream({}, { signal }),
      (response) => JSON.parse(new TextDecoder().decode(response.snapshotJson)) as RedisStats,
    ),
  ),
)

export function useRedisStatsStreamStatus() {
  return useStreamStatus(redisStatsStream)
}

export function useRedisStats() {
  useStream(redisStatsStream, queryKeys.redisStats)

  return useQuery({
    queryKey: queryKeys.redisStats,
    queryFn: async () => {
      const { snapshotJson } = await statsClient.get({})
      return JSON.parse(new TextDecoder().decode(snapshotJson)) as RedisStats
    },
    staleTime: Infinity,
  })
}

export function useLoggingConfig() {
  return useQuery({
    queryKey: queryKeys.logging,
    queryFn: async () => {
      const config = (await loggingClient.getConfig({})).config
      return {
        server_level: config?.serverLevel ?? '',
        sink: config?.sink ?? '',
        endpoint: config?.endpoint ?? '',
        stream: config?.stream ?? '',
      } satisfies LoggingConfig
    },
  })
}

// Same opaque-JSON pattern as redisStatsStream above.
const logTailStream = registerStream(
  new Stream((signal) =>
    mapAsync(
      loggingClient.streamTail({}, { signal }),
      (response) => JSON.parse(new TextDecoder().decode(response.recordsJson)) as LogTailEntry[],
    ),
  ),
)

export function useLogTailStreamStatus() {
  return useStreamStatus(logTailStream)
}

// The live tail streams continuously; the queryFn covers first paint and a
// down stream.
export function useLogTail() {
  useStream(logTailStream, queryKeys.logTail)

  return useQuery({
    queryKey: queryKeys.logTail,
    queryFn: async () => {
      const { recordsJson } = await loggingClient.getTail({})
      return JSON.parse(new TextDecoder().decode(recordsJson)) as LogTailEntry[]
    },
    staleTime: Infinity,
  })
}

// A dedicated sub-resource rather than a full AgentConfig upsert: setting a
// level should never risk touching an agent's mappings, and it works even for
// an agent that isn't configured with any yet (the server creates a bare
// entry) — see SetAgentLogLevel's doc comment on the Go side.
export function useSetAgentLogLevel() {
  return useConfigMutation<{ slug: string; log_level: string }, ConnectAcceptedResponse>(
    ({ slug, log_level }) => agentClient.setLogLevel({ slug, logLevel: log_level }),
    [queryKeys.config, queryKeys.agents],
  )
}

/*
 * Writes. Each invalidates every query that could show the change: the config
 * document overlaps the scoped endpoints, so a scan directory edit has to
 * refresh both its own list and the whole-config read.
 */

type Accepted = AcceptedResponse

// TResult defaults to the REST-era Accepted shape but any still-migrating
// domain's mutationFn can return its own type instead — e.g. a gRPC-Web
// domain returning the generated metarr.v1.AcceptedResponse message
// (camelCase correlationId, not snake_case correlation_id) rather than
// shimming one shape into the other. Nothing downstream reads these fields
// today; onSuccess only triggers invalidation.
function useConfigMutation<TVariables, TResult = Accepted>(
  mutationFn: (variables: TVariables) => Promise<TResult>,
  keysToInvalidate: readonly (readonly unknown[])[],
): UseMutationResult<TResult, Error, TVariables> {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn,
    onSuccess: () => {
      keysToInvalidate.forEach((key) => {
        void queryClient.invalidateQueries({ queryKey: key })
      })
    },
  })
}

export function useUpdateAdmin() {
  return useConfigMutation<
    MessageInitShape<typeof ConfigServiceUpdateAdminRequestSchema>,
    ConnectAcceptedResponse
  >((body) => configClient.updateAdmin(body), [queryKeys.config])
}

export function useUpsertApiKey() {
  return useConfigMutation<
    MessageInitShape<typeof ConfigServiceUpsertApiKeyRequestSchema>,
    ConnectAcceptedResponse
  >((body) => configClient.upsertApiKey(body), [queryKeys.config])
}

export function useDeleteApiKey() {
  return useConfigMutation<
    MessageInitShape<typeof ConfigServiceDeleteApiKeyRequestSchema>,
    ConnectAcceptedResponse
  >((body) => configClient.deleteApiKey(body), [queryKeys.config])
}

// A single upsert POST, like the other newer config sections — see the
// upsert-not-PUT convention in CLAUDE.md.
export function useUpdateLoggingConfig() {
  return useConfigMutation<LoggingConfig, ConnectAcceptedResponse>(
    (body) =>
      loggingClient.updateConfig({
        config: {
          serverLevel: body.server_level,
          sink: body.sink,
          endpoint: body.endpoint,
          stream: body.stream,
        },
      }),
    [queryKeys.logging, queryKeys.config],
  )
}

export function useUpdateDirectoryScanner() {
  return useConfigMutation<UpdateDirectoryScannerRequest, ConnectAcceptedResponse>(
    (body) => directoryScannerClient.update({ parallelCount: body.parallel_count }),
    [queryKeys.directoryScanner, queryKeys.config],
  )
}

export function useUpsertScanDirectory() {
  return useConfigMutation<ScanDirectory, ConnectAcceptedResponse>(
    (body) =>
      directoryScannerClient.upsertDirectory({
        directory: {
          scannerSlug: body.scanner_slug,
          scanType: body.scan_type,
          directory: body.directory,
        },
      }),
    [queryKeys.scanDirectories, queryKeys.directoryScanner, queryKeys.config],
  )
}

export function useDeleteScanDirectory() {
  return useConfigMutation<string, ConnectAcceptedResponse>(
    (slug) => directoryScannerClient.deleteDirectory({ slug }),
    [queryKeys.scanDirectories, queryKeys.directoryScanner, queryKeys.config],
  )
}

export function useUpsertSidecarType() {
  return useConfigMutation<SidecarTypeDefinition, ConnectAcceptedResponse>(
    (body) =>
      directoryScannerClient.upsertSidecarType({
        type: {
          id: body.id,
          type: body.type,
          category: body.category,
          order: body.order,
          patterns: body.patterns,
          extensions: body.extensions,
        },
      }),
    [queryKeys.sidecarTypes, queryKeys.directoryScanner, queryKeys.config],
  )
}

export function useDeleteSidecarType() {
  return useConfigMutation<string, ConnectAcceptedResponse>(
    (id) => directoryScannerClient.deleteSidecarType({ id }),
    [queryKeys.sidecarTypes, queryKeys.directoryScanner, queryKeys.config],
  )
}

// Ordering covers the whole table in one call — it is the only place an entry
// can be enabled or disabled, since order zero is the disabled sentinel.
export function useReorderSidecarTypes() {
  return useConfigMutation<ReorderSidecarTypesRequest, ConnectAcceptedResponse>(
    (orders) => directoryScannerClient.reorderSidecarTypes({ orders }),
    [queryKeys.sidecarTypes, queryKeys.directoryScanner, queryKeys.config],
  )
}

export function useResetSidecarTypes() {
  return useConfigMutation<void, ConnectAcceptedResponse>(
    () => directoryScannerClient.resetSidecarTypes({}),
    [queryKeys.sidecarTypes, queryKeys.directoryScanner, queryKeys.config],
  )
}

export function useUpsertSonarrInstance() {
  return useConfigMutation<MessageInitShape<typeof SonarrInstanceSchema>, ConnectAcceptedResponse>(
    (instance) => sonarrInterfaceClient.upsert({ instance }),
    [queryKeys.sonarr, queryKeys.config],
  )
}

export function useDeleteSonarrInstance() {
  return useConfigMutation<string, ConnectAcceptedResponse>(
    (slug) => sonarrInterfaceClient.delete({ slug }),
    [queryKeys.sonarr, queryKeys.config],
  )
}

/*
 * Workflows. Unlike everything above, these are a direct, synchronous Mongo
 * read/write with no config_update event behind them — see the Go handler's
 * doc comment on UpsertWorkflow — so they get their own mutation hook rather
 * than useConfigMutation, whose return type is hardwired to AcceptedResponse.
 */

// Nodes/edges/viewport travel as one opaque graph_json blob on the wire
// (see workflows.proto's doc comment — mongostore.Workflow itself has always
// stored them as loose bson.M, never a typed model), decoded back into the
// same flat Workflow shape types.ts already had so WorkflowEditorPage,
// WorkflowCanvas etc. need no changes.
function connectWorkflowToLegacyShape(w: ConnectWorkflow): Workflow {
  const graph = JSON.parse(new TextDecoder().decode(w.graphJson)) as {
    nodes: GraphNode[]
    edges: GraphEdge[]
    viewport: Record<string, unknown>
  }
  return {
    id: w.id,
    document_id: w.documentId,
    version: w.version,
    created_at: w.createdAt ? timestampDate(w.createdAt).toISOString() : new Date(0).toISOString(),
    name: w.name,
    description: w.description,
    tags: w.tags,
    schema_version: w.schemaVersion,
    nodes: graph.nodes,
    edges: graph.edges,
    viewport: graph.viewport,
  }
}

export function useWorkflow(id: string) {
  return useQuery({
    queryKey: queryKeys.workflow(id),
    queryFn: async () => {
      const { workflow } = await workflowClient.get({ id })
      return workflow && connectWorkflowToLegacyShape(workflow)
    },
    enabled: id !== '',
  })
}

export function useWorkflowVersions(id: string) {
  return useQuery({
    queryKey: queryKeys.workflowVersions(id),
    queryFn: async () =>
      (await workflowClient.listVersions({ id })).versions.map(connectWorkflowToLegacyShape),
    enabled: id !== '',
  })
}

export function useWorkflowVersion(id: string, version: number | null) {
  return useQuery({
    queryKey: [...queryKeys.workflow(id), 'v', version],
    queryFn: async () => {
      const { workflow } = await workflowClient.getVersion({ id, version: version ?? 0 })
      return workflow && connectWorkflowToLegacyShape(workflow)
    },
    enabled: id !== '' && version != null,
  })
}

// Infinite-scroll list, paginated by the opaque cursor List returns.
export function useWorkflowList() {
  return useInfiniteQuery({
    queryKey: queryKeys.workflows,
    queryFn: async ({ pageParam }: { pageParam: string | undefined }) => {
      const response = await workflowClient.list({ limit: 20, cursor: pageParam ?? '' })
      return {
        workflows: response.workflows.map(connectWorkflowToLegacyShape),
        next_cursor: response.nextCursor,
        has_more: response.hasMore,
      } satisfies WorkflowListResponse
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) =>
      lastPage.has_more ? lastPage.next_cursor : undefined,
  })
}

// The node/socket/transform catalog the palette, the node renderers, and
// isValidConnection all read from. staleTime: Infinity like useAgents/
// useConfig: it only changes on a server redeploy, never mid-session, so
// there's no socket topic behind it — just a plain fetch-once query.
export function useWorkflowCatalog() {
  return useQuery({
    queryKey: queryKeys.workflowCatalog,
    queryFn: async () => {
      const { catalogJson } = await workflowCatalogClient.get({})
      return JSON.parse(new TextDecoder().decode(catalogJson)) as CatalogResponse
    },
    staleTime: Infinity,
  })
}

export function useSaveWorkflow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (body: UpsertWorkflowRequest) => {
      const graphJson = new TextEncoder().encode(
        JSON.stringify({ nodes: body.nodes, edges: body.edges, viewport: body.viewport }),
      )
      const { workflow } = await workflowClient.upsert({
        documentId: body.document_id ?? '',
        name: body.name,
        description: body.description,
        tags: body.tags,
        schemaVersion: body.schema_version,
        graphJson,
      })
      if (!workflow) throw new Error('save did not return a workflow')
      return connectWorkflowToLegacyShape(workflow)
    },
    onSuccess: (saved) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.workflows })
      void queryClient.invalidateQueries({
        queryKey: queryKeys.workflow(saved.document_id),
      })
      void queryClient.invalidateQueries({
        queryKey: queryKeys.workflowVersions(saved.document_id),
      })
    },
  })
}
