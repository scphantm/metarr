import { useEffect, useRef, useState } from 'react'

import { request } from '../../api/client'
import { useDebouncedValue } from '../../lib/useDebouncedValue'
import type { Graph, ValidateResponse } from './catalogTypes'

const emptyResult: ValidateResponse = { diagnostics: [], runnable: true }

/*
 * Drives the debounced POST /api/workflows/validate call — design.md §6.6:
 * the client does its own cheap local checks during a drag
 * (connectionRules.ts), but the whole-graph analyses live only on the
 * server, so this is the authoritative check, called 500ms after the graph
 * stops changing. A request-id guard discards a response that resolves
 * after a newer request has already gone out, so a fast typist's stale
 * result can never clobber a fresher one.
 */
export function useWorkflowValidation(graph: Graph | null, enabled: boolean): ValidateResponse {
  const debouncedGraph = useDebouncedValue(graph, 500)
  const [result, setResult] = useState<ValidateResponse>(emptyResult)
  const requestId = useRef(0)

  useEffect(() => {
    if (!enabled || !debouncedGraph) return
    const id = ++requestId.current
    request<ValidateResponse>('/api/workflows/validate', {
      method: 'POST',
      body: { graph: debouncedGraph },
    })
      .then((response) => {
        if (id === requestId.current) setResult(response)
      })
      .catch(() => {
        // Best-effort — a failed validate call leaves diagnostics stale
        // until the next graph change. It must never block editing.
      })
  }, [debouncedGraph, enabled])

  // Computed rather than reset via a second setState call in the effect
  // above (which would trigger cascading renders): when disabled, the
  // caller gets the empty result directly instead of stale state lingering
  // from before it was disabled.
  return enabled && debouncedGraph ? result : emptyResult
}
