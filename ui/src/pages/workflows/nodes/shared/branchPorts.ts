import { useEffect, useMemo } from 'react'
import { useNodeConnections, useUpdateNodeInternals } from '@xyflow/react'

import { parseHandleId } from '../../connectionRules'
import type { WorkflowNodeType as NodeType } from '../../../../gen/metarr/v1/workflow_catalog_pb'
import type { CatalogNodeData } from '../../editorNodeData'
import { settingDefault } from '../../catalogValue'
import type { ArrangedHandle } from './useNodeHandles'

/*
 * Parallel's control-out ports and Join's control-in ports are both
 * declared in the catalog as the fixed set branch1..branch8 (the max arity
 * either node supports, design.md §5.2) but only as many as the node's own
 * `branches` setting calls for should actually be shown — the rest are
 * catalog headroom, not meant to clutter every node at full width. If an
 * edge is already wired to a branch beyond that count (the setting was
 * lowered after wiring, or a graph was authored by hand), that branch stays
 * visible regardless — React Flow can't draw an edge onto a handle that
 * isn't rendered.
 */

const BRANCH_PORT = /^branch(\d+)$/

function branchPortIndex(port: string): number | null {
  const match = BRANCH_PORT.exec(port)
  return match ? Number(match[1]) : null
}

export function limitBranchPorts(handles: ArrangedHandle[], visibleCount: number): ArrangedHandle[] {
  return handles.filter((handle) => {
    const index = branchPortIndex(handle.label)
    return index === null || index <= visibleCount
  })
}

function declaredBranches(data: CatalogNodeData, nodeType: NodeType | undefined): number {
  const fromSettings = data.settings.branches
  if (typeof fromSettings === 'number' && Number.isFinite(fromSettings)) return fromSettings
  const setting = nodeType?.settings.find((entry) => entry.name === 'branches')
  const fallback = settingDefault(setting?.default)
  return typeof fallback === 'number' ? fallback : 1
}

function maxBranchIndex(ports: string[]): number {
  return ports.reduce((max, port) => Math.max(max, branchPortIndex(port) ?? 0), 0)
}

// How many branchN handles a Parallel/Join node should render right now:
// whichever is larger of its declared `branches` setting and the highest
// branch index some edge is already wired to, capped at what the catalog
// declares for that direction.
export function useVisibleBranchCount(
  nodeId: string,
  data: CatalogNodeData,
  nodeType: NodeType | undefined,
  ports: string[],
  handleType: 'source' | 'target',
): number {
  const connections = useNodeConnections({ id: nodeId, handleType })
  const updateNodeInternals = useUpdateNodeInternals()

  const visibleCount = useMemo(() => {
    const declared = declaredBranches(data, nodeType)
    const wiredMax = connections.reduce((max, connection) => {
      const handleId = handleType === 'source' ? connection.sourceHandle : connection.targetHandle
      const parsed = parseHandleId(handleId)
      const index = parsed?.kind === 'control' ? branchPortIndex(parsed.name) : null
      return index !== null && index > max ? index : max
    }, 0)
    return Math.min(maxBranchIndex(ports), Math.max(declared, wiredMax, 1))
  }, [data, nodeType, connections, handleType, ports])

  // React Flow measures each handle's connection-anchor position once and
  // caches it — it does not re-measure just because a re-render changes
  // which/how many <Handle> elements a node mounts. Without this, the dot
  // moves (plain DOM/CSS) but drags and edge paths keep snapping to the
  // stale position. Must run after the new handles have committed to the
  // DOM, hence useEffect rather than doing this inside the useMemo above.
  useEffect(() => {
    updateNodeInternals(nodeId)
  }, [nodeId, visibleCount, updateNodeInternals])

  return visibleCount
}
