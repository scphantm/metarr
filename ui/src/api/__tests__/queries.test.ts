import { describe, it, expect, vi } from 'vitest'
import { createElement, type ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'

const purge = vi.fn()

vi.mock('../clients', () => ({
  statsClient: { purge: (...args: unknown[]) => purge(...args) },
  // queries.ts pulls the rest of the clients in at module load; the purge
  // hook is the only one under test here, so the others just need to exist.
  agentClient: {},
  configClient: {},
  directoryScannerClient: {},
  eventBusClient: {},
  loggingClient: {},
  sonarrInterfaceClient: {},
  workflowCatalogClient: {},
  workflowClient: {},
}))

import { queryKeys, usePurgeStreams } from '../queries'

describe('queryKeys', () => {
  describe('static keys', () => {
    it('defines config key', () => {
      expect(queryKeys.config).toEqual(['config'])
    })

    it('defines nested config keys', () => {
      expect(queryKeys.directoryScanner).toEqual(['config', 'directory-scanner'])
      expect(queryKeys.scanDirectories).toEqual(['config', 'scan-directories'])
      expect(queryKeys.sidecarTypes).toEqual(['config', 'sidecar-types'])
    })

    it('defines stats keys', () => {
      expect(queryKeys.busSnapshot).toEqual(['stats', 'bus'])
      expect(queryKeys.agents).toEqual(['stats', 'agents'])
      expect(queryKeys.logTail).toEqual(['stats', 'log-tail'])
    })

    it('defines workflow catalog key', () => {
      expect(queryKeys.workflowCatalog).toEqual(['workflows', 'catalog'])
      expect(queryKeys.workflows).toEqual(['workflows'])
    })
  })

  describe('dynamic keys', () => {
    it('generates workflow keys by id', () => {
      const workflowId = 'workflow-456'
      expect(queryKeys.workflow(workflowId)).toEqual(['workflows', workflowId])
      expect(queryKeys.workflowVersions(workflowId)).toEqual([
        'workflows',
        workflowId,
        'versions',
      ])
    })

    it('handles different ids independently', () => {
      expect(queryKeys.workflow('id-a')).not.toEqual(queryKeys.workflow('id-b'))
    })
  })
})

describe('usePurgeStreams', () => {
  function harness() {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children)
    return { queryClient, invalidate, wrapper }
  }

  it('calls StatsService.Purge with a single stream name', async () => {
    purge.mockReset().mockResolvedValue({ results: [] })
    const { wrapper } = harness()

    const { result } = renderHook(() => usePurgeStreams(), { wrapper })
    result.current.mutate({ stream: 'events.agent_scan_results' })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(purge).toHaveBeenCalledWith({ stream: 'events.agent_scan_results' })
  })

  it('calls StatsService.Purge with the all flag', async () => {
    purge.mockReset().mockResolvedValue({ results: [] })
    const { wrapper } = harness()

    const { result } = renderHook(() => usePurgeStreams(), { wrapper })
    result.current.mutate({ all: true })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(purge).toHaveBeenCalledWith({ all: true })
  })

  it('invalidates the bus snapshot on success so the drained depth shows', async () => {
    purge.mockReset().mockResolvedValue({ results: [] })
    const { invalidate, wrapper } = harness()

    const { result } = renderHook(() => usePurgeStreams(), { wrapper })
    result.current.mutate({ stream: 'events.agent_scan_results' })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(invalidate).toHaveBeenCalledWith({ queryKey: queryKeys.busSnapshot })
  })
})
