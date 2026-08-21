import type { Node, NodeProps } from '@xyflow/react'

import { NodeShell } from '../shared/NodeShell'
import { limitBranchPorts, useVisibleBranchCount } from '../shared/branchPorts'
import { useCatalogEntry } from '../../useCatalogEntry'
import { useNodeHandles } from '../shared/useNodeHandles'
import type { CatalogNodeData } from '../../catalogTypes'

const TYPE_KEY = 'core/join'

// Only as many branchN control-in ports render as the node's `branches`
// setting calls for (plus any branch already wired past that) — mirrors
// Parallel's control-out ports, see nodes/shared/branchPorts.ts.
export function JoinNode({ id, data }: NodeProps<Node<CatalogNodeData>>) {
  const nodeType = useCatalogEntry(data.catalogId, TYPE_KEY)
  const handles = useNodeHandles(nodeType)
  const visibleBranches = useVisibleBranchCount(id, data, nodeType, nodeType?.control.in ?? [], 'target')

  return (
    <NodeShell
      id={id}
      data={data}
      typeKey={TYPE_KEY}
      handles={{ ...handles, top: limitBranchPorts(handles.top, visibleBranches) }}
    />
  )
}
