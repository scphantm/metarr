import type { Node, NodeProps } from '@xyflow/react'

import { NodeShell } from '../shared/NodeShell'
import { limitBranchPorts, useVisibleBranchCount } from '../shared/branchPorts'
import { useCatalogEntry } from '../../useCatalogEntry'
import { useNodeHandles } from '../shared/useNodeHandles'
import type { CatalogNodeData } from '../../editorNodeData'

const TYPE_KEY = 'core/parallel'

// Only as many branchN control-out ports render as the node's `branches`
// setting calls for (plus any branch already wired past that) — see
// nodes/shared/branchPorts.ts.
export function ParallelNode({ id, data }: NodeProps<Node<CatalogNodeData>>) {
  const nodeType = useCatalogEntry(data.catalogId, TYPE_KEY)
  const handles = useNodeHandles(nodeType)
  const visibleBranches = useVisibleBranchCount(id, data, nodeType, nodeType?.control?.out ?? [], 'source')

  return (
    <NodeShell
      id={id}
      data={data}
      typeKey={TYPE_KEY}
      handles={{ ...handles, bottom: limitBranchPorts(handles.bottom, visibleBranches) }}
    />
  )
}
