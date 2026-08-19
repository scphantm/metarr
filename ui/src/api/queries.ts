import {
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
  RedisStats,
  ReorderSidecarTypesRequest,
  ScanDirectory,
  SidecarTypeDefinition,
  SonarrInstance,
  UpdateAdminRequest,
  UpdateDirectoryScannerRequest,
} from './types'

export const queryKeys = {
  config: ['config'] as const,
  directoryScanner: ['config', 'directory-scanner'] as const,
  scanDirectories: ['config', 'scan-directories'] as const,
  sidecarTypes: ['config', 'sidecar-types'] as const,
  sonarr: ['config', 'interfaces', 'sonarr'] as const,
  // Deliberately outside the config tree: this one is fed by a socket, and
  // the config-wide invalidations should not reach it.
  redisStats: ['stats', 'redis'] as const,
  agents: ['stats', 'agents'] as const,
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
