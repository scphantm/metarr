import { BaseEdge, getBezierPath, type Edge, type EdgeProps } from '@xyflow/react'

import { controlHandleId } from '../connectionRules'

/*
 * The one shared control-edge component — edges aren't declared per catalog
 * entry, the catalog only distinguishes control vs data via Edge.kind, so
 * unlike the node components this stays a single component for every
 * control connection. Reworks the vendored animated-svg-edge.tsx (deleted):
 * thick, solid, animated dot, colored from the source handle rather than the
 * old hardcoded #ff0073 — red when it's the error branch, the ordinary ink
 * tone otherwise.
 */

export type ControlEdgeType = Edge<Record<string, never>, 'controlEdge'>

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
}: EdgeProps<ControlEdgeType>) {
  const [path] = getBezierPath({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition })
  const color = sourceHandleId === ERROR_HANDLE ? 'var(--color-red)' : 'var(--color-ink-strong)'

  return (
    <>
      <BaseEdge id={id} path={path} markerEnd={markerEnd} style={{ stroke: color, strokeWidth: 3 }} />
      <circle r="4" fill={color}>
        <animateMotion dur="2s" repeatCount="indefinite" path={path} calcMode="linear" />
      </circle>
    </>
  )
}
