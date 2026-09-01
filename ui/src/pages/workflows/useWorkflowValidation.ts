import { useEffect, useRef, useState } from 'react'

import { workflowCatalogClient } from '../../api/clients'
import type { WorkflowCatalogServiceValidateResponse } from '../../gen/metarr/v1/workflow_catalog_pb'
import type { WorkflowGraph } from '../../gen/metarr/v1/workflow_graph_pb'
import { useDebouncedValue } from '../../lib/useDebouncedValue'

// The hook's return shape is the generated validate response itself, narrowed
// to the two fields the editor reads — WorkflowDiagnostic flows through
// unchanged, so there is no hand-written mirror of it (docs/adr/0005).
type ValidationResult = Pick<
  WorkflowCatalogServiceValidateResponse,
  'diagnostics' | 'runnable'
>

const emptyResult: ValidationResult = { diagnostics: [], runnable: true }

/*
 * Drives the debounced POST /api/workflows/validate call — design.md §6.6:
 * the client does its own cheap local checks during a drag
 * (connectionRules.ts), but the whole-graph analyses live only on the
 * server, so this is the authoritative check, called 500ms after the graph
 * stops changing. A request-id guard discards a response that resolves
 * after a newer request has already gone out, so a fast typist's stale
 * result can never clobber a fresher one.
 */
export function useWorkflowValidation(
  graph: WorkflowGraph | null,
  enabled: boolean,
): ValidationResult {
  const debouncedGraph = useDebouncedValue(graph, 500)
  const [result, setResult] = useState<ValidationResult>(emptyResult)
  const requestId = useRef(0)

  useEffect(() => {
    if (!enabled || !debouncedGraph) return
    const id = ++requestId.current
    workflowCatalogClient
      .validate({ graph: debouncedGraph })
      .then((response) => {
        if (id !== requestId.current) return
        setResult({
          diagnostics: response.diagnostics,
          runnable: response.runnable,
        })
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
