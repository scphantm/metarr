import { useMemo } from 'react'

import { controlHandleId, dataHandleId, type Type } from '../../connectionRules'
import type { WorkflowNodeType as NodeType } from '../../../../gen/metarr/v1/workflow_catalog_pb'

/*
 * Arranges one catalog NodeType's ports into the top/bottom/error layout
 * CLAUDE.md's "Workflow UI Node Design Pattern" defines: top = control-in
 * ports + data-in sockets, bottom = control-out ports (excluding error) +
 * data-out sockets, error on the side. `kind`/`control` drives which ports
 * exist — never a hardcoded category check.
 */

export type ArrangedHandle = {
  id: string
  label: string
  kind: 'control' | 'data'
  type?: Type
  // Full hover text for the handle — built once here so every node file
  // renders the same wording instead of re-deriving it per component.
  title: string
}

export type ArrangedHandles = {
  top: ArrangedHandle[]
  bottom: ArrangedHandle[]
  hasError: boolean
}

const emptyHandles: ArrangedHandles = { top: [], bottom: [], hasError: false }

// Hover text for the error handle is identical on every node type — it's
// not sourced from the catalog — so it's a constant rather than something
// useNodeHandles computes per call.
export const errorHandleTitle =
  'Error — control flow taken when this node fails'

function controlTitle(direction: 'in' | 'out', port: string): string {
  return direction === 'in' ? `Control in — ${port}` : `Control out — ${port}`
}

function dataTitle(
  direction: 'in' | 'out',
  socket: {
    label?: string
    name: string
    type: Type
    required?: boolean
    description?: string
  },
): string {
  const label = socket.label || socket.name
  const parts = [
    `Data ${direction} — ${label}: ${socket.type}${socket.required ? ' (required)' : ''}`,
  ]
  if (socket.description) parts.push(socket.description)
  return parts.join(' — ')
}

export function useNodeHandles(
  nodeType: NodeType | undefined,
): ArrangedHandles {
  return useMemo(() => {
    if (!nodeType) return emptyHandles

    const top: ArrangedHandle[] = [
      ...(nodeType.control?.in ?? []).map((port) => ({
        id: controlHandleId(port),
        label: port,
        kind: 'control' as const,
        title: controlTitle('in', port),
      })),
      ...nodeType.dataIn.map((socket) => ({
        id: dataHandleId(socket.name),
        label: socket.label || socket.name,
        kind: 'data' as const,
        type: socket.type,
        title: dataTitle('in', socket),
      })),
    ]

    const bottom: ArrangedHandle[] = [
      ...(nodeType.control?.out ?? []).map((port) => ({
        id: controlHandleId(port),
        label: port,
        kind: 'control' as const,
        title: controlTitle('out', port),
      })),
      ...nodeType.dataOut.map((socket) => ({
        id: dataHandleId(socket.name),
        label: socket.label || socket.name,
        kind: 'data' as const,
        type: socket.type,
        title: dataTitle('out', socket),
      })),
    ]

    return { top, bottom, hasError: Boolean(nodeType.control?.error) }
  }, [nodeType])
}

// Evenly spaces `total` handles along one edge of the node, leaving equal
// margins before the first and after the last rather than running edge to
// edge. Kept from the old nodeSockets.tsx — the one piece of that file that
// stayed useful under the new catalog-driven model.
export function handleOffset(index: number, total: number): string {
  return `${((index + 1) / (total + 1)) * 100}%`
}
