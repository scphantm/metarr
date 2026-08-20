import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
} from '@tanstack/react-query'

import { request } from './client'
import { useTopic } from './useTopic'
import type {
  AcceptedResponse,
  AgentConfig,
  AgentView,
  Config,
  DirectoryScannerConfig,
  LoggingConfig,
  LogTailEntry,
  RedisStats,
  ReorderSidecarTypesRequest,
  ScanDirectory,
  SidecarTypeDefinition,
  SonarrInstance,
  UpdateAdminRequest,
  UpdateDirectoryScannerRequest,
  UpsertWorkflowRequest,
  Workflow,
  WorkflowListResponse,
} from './types'

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
  // Also outside the config tree: workflows are a server-only, single-
  // collection concern with no config-mutation event behind them at all.
  workflows: ['workflows'] as const,
  workflow: (id: string) => ['workflows', id] as const,
  workflowVersions: (id: string) => ['workflows', id, 'versions'] as const,
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
    queryFn: () => request<Config>('/api/config'),
  })
}

export function useDirectoryScannerConfig() {
  return useQuery({
    queryKey: queryKeys.directoryScanner,
    queryFn: () =>
      request<DirectoryScannerConfig>('/api/config/directory-scanner'),
  })
}

export function useScanDirectories() {
  return useQuery({
    queryKey: queryKeys.scanDirectories,
    queryFn: () =>
      request<ScanDirectory[]>('/api/config/directory-scanner/directories'),
  })
}

export function useSidecarTypes() {
  return useQuery({
    queryKey: queryKeys.sidecarTypes,
    queryFn: () =>
      request<SidecarTypeDefinition[]>(
        '/api/config/directory-scanner/sidecar-types',
      ),
  })
}

export function useSonarrInstances() {
  return useQuery({
    queryKey: queryKeys.sonarr,
    queryFn: () => request<SonarrInstance[]>('/api/config/interfaces/sonarr'),
  })
}

// Redis statistics arrive over the socket rather than by refetching: useTopic
// writes each frame straight into this query's cache entry. The queryFn still
// runs once, so the page has something to paint before the socket is up and
// something to fall back on if it never connects.
// Agents stream over the socket for the same reason the Redis stats do: the
// telemetry is live and the presence half changes on its own, with no user
// action to hang a refetch off. The queryFn covers the first paint.
export function useAgents() {
  useTopic('agents.presence', queryKeys.agents)

  return useQuery({
    queryKey: queryKeys.agents,
    queryFn: () => request<AgentView[]>('/api/config/agents'),
    staleTime: Infinity,
  })
}

// Agents are upserted by slug, like every other config collection here.
export function useUpsertAgent() {
  return useConfigMutation<AgentConfig>(
    (body) => request<Accepted>('/api/config/agents', { method: 'POST', body }),
    [queryKeys.config, queryKeys.agents],
  )
}

export function useDeleteAgent() {
  return useConfigMutation<string>(
    (slug) =>
      request<Accepted>(`/api/config/agents/${encodeURIComponent(slug)}`, {
        method: 'DELETE',
      }),
    [queryKeys.config, queryKeys.agents],
  )
}

export function useRedisStats() {
  useTopic('stats.redis', queryKeys.redisStats)

  return useQuery({
    queryKey: queryKeys.redisStats,
    queryFn: () => request<RedisStats>('/api/stats/redis'),
    staleTime: Infinity,
  })
}

export function useLoggingConfig() {
  return useQuery({
    queryKey: queryKeys.logging,
    queryFn: () => request<LoggingConfig>('/api/config/logging'),
  })
}

// The live tail streams over the socket, same shape as useRedisStats/useAgents:
// the queryFn covers first paint and a down socket, the topic keeps it fresh.
export function useLogTail() {
  useTopic('logging.tail', queryKeys.logTail)

  return useQuery({
    queryKey: queryKeys.logTail,
    queryFn: () => request<LogTailEntry[]>('/api/logging/tail'),
    staleTime: Infinity,
  })
}

// A dedicated sub-resource rather than a full AgentConfig upsert: setting a
// level should never risk touching an agent's mappings, and it works even for
// an agent that isn't configured with any yet (the server creates a bare
// entry) — see SetAgentLogLevel's doc comment on the Go side.
export function useSetAgentLogLevel() {
  return useConfigMutation<{ slug: string; log_level: string }>(
    ({ slug, log_level }) =>
      request<Accepted>(
        `/api/config/agents/${encodeURIComponent(slug)}/log-level`,
        { method: 'POST', body: { log_level } },
      ),
    [queryKeys.config, queryKeys.agents],
  )
}

/*
 * Writes. Each invalidates every query that could show the change: the config
 * document overlaps the scoped endpoints, so a scan directory edit has to
 * refresh both its own list and the whole-config read.
 */

type Accepted = AcceptedResponse

function useConfigMutation<TVariables>(
  mutationFn: (variables: TVariables) => Promise<Accepted>,
  keysToInvalidate: readonly (readonly unknown[])[],
): UseMutationResult<Accepted, Error, TVariables> {
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
  return useConfigMutation<UpdateAdminRequest>(
    (body) =>
      request<Accepted>('/api/config/admin', { method: 'PUT', body }),
    [queryKeys.config],
  )
}

// The whole-document update, used for the API key groups, which have no
// endpoint of their own.
export function useUpdateConfig() {
  return useConfigMutation<Config>(
    (body) => request<Accepted>('/api/config', { method: 'PUT', body }),
    [queryKeys.config, queryKeys.sonarr, queryKeys.directoryScanner],
  )
}

// A single upsert POST, like the other newer config sections — see the
// upsert-not-PUT convention in CLAUDE.md.
export function useUpdateLoggingConfig() {
  return useConfigMutation<LoggingConfig>(
    (body) =>
      request<Accepted>('/api/config/logging', { method: 'POST', body }),
    [queryKeys.logging, queryKeys.config],
  )
}

export function useUpdateDirectoryScanner() {
  return useConfigMutation<UpdateDirectoryScannerRequest>(
    (body) =>
      request<Accepted>('/api/config/directory-scanner', {
        method: 'PUT',
        body,
      }),
    [queryKeys.directoryScanner, queryKeys.config],
  )
}

export function useUpsertScanDirectory() {
  return useConfigMutation<ScanDirectory>(
    (body) =>
      request<Accepted>('/api/config/directory-scanner/directories', {
        method: 'POST',
        body,
      }),
    [queryKeys.scanDirectories, queryKeys.directoryScanner, queryKeys.config],
  )
}

export function useDeleteScanDirectory() {
  return useConfigMutation<string>(
    (slug) =>
      request<Accepted>(
        `/api/config/directory-scanner/directories/${encodeURIComponent(slug)}`,
        { method: 'DELETE' },
      ),
    [queryKeys.scanDirectories, queryKeys.directoryScanner, queryKeys.config],
  )
}

export function useUpsertSidecarType() {
  return useConfigMutation<SidecarTypeDefinition>(
    (body) =>
      request<Accepted>('/api/config/directory-scanner/sidecar-types', {
        method: 'POST',
        body,
      }),
    [queryKeys.sidecarTypes, queryKeys.directoryScanner, queryKeys.config],
  )
}

export function useDeleteSidecarType() {
  return useConfigMutation<string>(
    (id) =>
      request<Accepted>(
        `/api/config/directory-scanner/sidecar-types/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
      ),
    [queryKeys.sidecarTypes, queryKeys.directoryScanner, queryKeys.config],
  )
}

// Ordering covers the whole table in one call — it is the only place an entry
// can be enabled or disabled, since order zero is the disabled sentinel.
export function useReorderSidecarTypes() {
  return useConfigMutation<ReorderSidecarTypesRequest>(
    (body) =>
      request<Accepted>('/api/config/directory-scanner/sidecar-types/order', {
        method: 'POST',
        body,
      }),
    [queryKeys.sidecarTypes, queryKeys.directoryScanner, queryKeys.config],
  )
}

export function useResetSidecarTypes() {
  return useConfigMutation<void>(
    () =>
      request<Accepted>('/api/config/directory-scanner/sidecar-types/reset', {
        method: 'POST',
      }),
    [queryKeys.sidecarTypes, queryKeys.directoryScanner, queryKeys.config],
  )
}

export function useUpsertSonarrInstance() {
  return useConfigMutation<SonarrInstance>(
    (body) =>
      request<Accepted>('/api/config/interfaces/sonarr', {
        method: 'POST',
        body,
      }),
    [queryKeys.sonarr, queryKeys.config],
  )
}

export function useDeleteSonarrInstance() {
  return useConfigMutation<string>(
    (slug) =>
      request<Accepted>(
        `/api/config/interfaces/sonarr/${encodeURIComponent(slug)}`,
        { method: 'DELETE' },
      ),
    [queryKeys.sonarr, queryKeys.config],
  )
}

/*
 * Workflows. Unlike everything above, these are a direct, synchronous Mongo
 * read/write with no config_update event behind them — see the Go handler's
 * doc comment on UpsertWorkflow — so they get their own mutation hook rather
 * than useConfigMutation, whose return type is hardwired to AcceptedResponse.
 */

export function useWorkflow(id: string) {
  return useQuery({
    queryKey: queryKeys.workflow(id),
    queryFn: () => request<Workflow>(`/api/workflows/${id}`),
    enabled: id !== '',
  })
}

export function useWorkflowVersions(id: string) {
  return useQuery({
    queryKey: queryKeys.workflowVersions(id),
    queryFn: () => request<Workflow[]>(`/api/workflows/${id}/versions`),
    enabled: id !== '',
  })
}

export function useWorkflowVersion(id: string, version: number | null) {
  return useQuery({
    queryKey: [...queryKeys.workflow(id), 'v', version],
    queryFn: () => request<Workflow>(`/api/workflows/${id}/versions/${version}`),
    enabled: id !== '' && version != null,
  })
}

// Infinite-scroll list, paginated by the opaque cursor ListWorkflows returns.
export function useWorkflowList() {
  return useInfiniteQuery({
    queryKey: queryKeys.workflows,
    queryFn: ({ pageParam }: { pageParam: string | undefined }) =>
      request<WorkflowListResponse>(
        `/api/workflows?limit=20${pageParam ? `&cursor=${encodeURIComponent(pageParam)}` : ''}`,
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) =>
      lastPage.has_more ? lastPage.next_cursor : undefined,
  })
}

export function useSaveWorkflow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: UpsertWorkflowRequest) =>
      request<Workflow>('/api/workflows', { method: 'POST', body }),
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
