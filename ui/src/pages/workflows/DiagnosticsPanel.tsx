import { useState } from 'react'

import type { Diagnostic } from './catalogTypes'

/*
 * Renders the debounced POST /api/workflows/validate result as a collapsible
 * list, docked inside the canvas via React Flow's own <Panel>. Clicking a
 * row (or a node in its witness path) pans/selects that node — see
 * WorkflowCanvas's onSelectDiagnosticNode.
 */
export function DiagnosticsPanel({
  diagnostics,
  nodeLabel,
  onSelectNode,
}: {
  diagnostics: Diagnostic[]
  nodeLabel: (nodeId: string) => string
  onSelectNode: (nodeId: string) => void
}) {
  const [open, setOpen] = useState(false)

  const errorCount = diagnostics.filter((d) => d.severity === 'error').length
  const warningCount = diagnostics.length - errorCount

  return (
    <div className="w-72 rounded-lg border border-edge bg-surface shadow-lg">
      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        className="flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-xs font-semibold text-ink-strong"
      >
        <span>Diagnostics</span>
        <span className="flex items-center gap-1.5">
          {errorCount > 0 ? (
            <span className="rounded-full bg-red/15 px-1.5 py-0.5 text-[10px] font-medium text-red">{errorCount}</span>
          ) : null}
          {warningCount > 0 ? (
            <span className="rounded-full bg-yellow/15 px-1.5 py-0.5 text-[10px] font-medium text-yellow">
              {warningCount}
            </span>
          ) : null}
          {diagnostics.length === 0 ? <span className="text-[10px] text-ink-muted">clean</span> : null}
          <span className="text-ink-muted">{open ? '▾' : '▸'}</span>
        </span>
      </button>

      {open ? (
        <div className="max-h-64 overflow-y-auto border-t border-edge/60">
          {diagnostics.length === 0 ? (
            <p className="px-3 py-3 text-xs text-ink-muted">No issues.</p>
          ) : (
            <ul className="flex flex-col divide-y divide-edge/60">
              {diagnostics.map((diagnostic, index) => (
                <li key={`${diagnostic.code}-${index}`} className="px-3 py-2">
                  <div className="flex items-start gap-1.5">
                    <span
                      className={`mt-0.5 h-1.5 w-1.5 shrink-0 rounded-full ${
                        diagnostic.severity === 'error' ? 'bg-red' : 'bg-yellow'
                      }`}
                    />
                    <div className="min-w-0 flex-1">
                      <p className="text-xs text-ink-strong">{diagnostic.message}</p>
                      {diagnostic.node_ids && diagnostic.node_ids.length > 0 ? (
                        <div className="mt-1 flex flex-wrap gap-1">
                          {diagnostic.node_ids.map((nodeId) => (
                            <button
                              key={nodeId}
                              type="button"
                              onClick={() => onSelectNode(nodeId)}
                              className="rounded border border-edge-strong/40 bg-canvas px-1.5 py-0.5 text-[10px] text-ink-strong hover:border-blue"
                            >
                              {nodeLabel(nodeId)}
                            </button>
                          ))}
                        </div>
                      ) : null}
                      {diagnostic.witness_path && diagnostic.witness_path.length > 0 ? (
                        <div className="mt-1 flex flex-wrap items-center gap-1 text-[10px] text-ink-muted">
                          {diagnostic.witness_path.map((nodeId, pathIndex) => (
                            <span key={`${nodeId}-${pathIndex}`} className="flex items-center gap-1">
                              {pathIndex > 0 ? <span>→</span> : null}
                              <button
                                type="button"
                                onClick={() => onSelectNode(nodeId)}
                                className="underline decoration-dotted hover:text-blue"
                              >
                                {nodeLabel(nodeId)}
                              </button>
                            </span>
                          ))}
                        </div>
                      ) : null}
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : null}
    </div>
  )
}
