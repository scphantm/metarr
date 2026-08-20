import { useMemo } from 'react'

import { controlHandleId, dataHandleId } from '../../connectionRules'
import type { NodeType, Type } from '../../catalogTypes'

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
}

export type ArrangedHandles = {
  top: ArrangedHandle[]
  bottom: ArrangedHandle[]
  hasError: boolean
}

const emptyHandles: ArrangedHandles = { top: [], bottom: [], hasError: false }

export function useNodeHandles(nodeType: NodeType | undefined): ArrangedHandles {
  return useMemo(() => {
    if (!nodeType) return emptyHandles

    const top: ArrangedHandle[] = [
      ...nodeType.control.in.map((port) => ({ id: controlHandleId(port), label: port, kind: 'control' as const })),
      ...(nodeType.dataIn ?? []).map((socket) => ({
        id: dataHandleId(socket.name),
        label: socket.label ?? socket.name,
        kind: 'data' as const,
        type: socket.type,
      })),
    ]

    const bottom: ArrangedHandle[] = [
      ...nodeType.control.out.map((port) => ({ id: controlHandleId(port), label: port, kind: 'control' as const })),
      ...(nodeType.dataOut ?? []).map((socket) => ({
        id: dataHandleId(socket.name),
        label: socket.label ?? socket.name,
        kind: 'data' as const,
        type: socket.type,
      })),
    ]

    return { top, bottom, hasError: Boolean(nodeType.control.error) }
  }, [nodeType])
}

// Evenly spaces `total` handles along one edge of the node, leaving equal
// margins before the first and after the last rather than running edge to
// edge. Kept from the old nodeSockets.tsx — the one piece of that file that
// stayed useful under the new catalog-driven model.
export function handleOffset(index: number, total: number): string {
  return `${((index + 1) / (total + 1)) * 100}%`
}
