import { BaseEdge, getBezierPath, type Edge, type EdgeProps } from '@xyflow/react'

import { controlHandleId } from '../connectionRules'

/*
 * The one shared control-edge component — edges aren't declared per catalog
 * entry, the catalog only distinguishes control vs data via Edge.kind, so
 * unlike the node components this stays a single component for every
 * control connection. Reworks the vendored animated-svg-edge.tsx (deleted):
 * thick, solid, animated dot, colored from the source handle rather than the
 * old hardcoded #ff0073 — cyan for an ordinary control edge, red when it
 * leaves the error branch (the color legend's own rule: edges connecting to
 * an error port are always theme red, regardless of edge kind).
 */

// diagnosticHighlight is set by WorkflowCanvas.tsx while the user hovers a
// diagnostic naming this edge — see DiagnosticsPanel.tsx — never persisted
// (graphAdapter.ts's fromRFEdge doesn't read it).
export type ControlEdgeData = { diagnosticHighlight?: boolean }
export type ControlEdgeType = Edge<ControlEdgeData, 'controlEdge'>

const ERROR_HANDLE = controlHandleId('error')

export function ControlEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  sourceHandleId,
  markerEnd,
  data,
}: EdgeProps<ControlEdgeType>) {
  const [path] = getBezierPath({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition })
  const color = sourceHandleId === ERROR_HANDLE ? 'var(--color-red)' : 'var(--color-cyan)'

  return (
    <g className={data?.diagnosticHighlight ? 'diagnostic-blink' : undefined}>
      <BaseEdge id={id} path={path} markerEnd={markerEnd} style={{ stroke: color, strokeWidth: 3 }} />
      <circle r="4" fill={color}>
        <animateMotion dur="2s" repeatCount="indefinite" path={path} calcMode="linear" />
      </circle>
    </g>
  )
}
