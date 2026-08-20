import type { Node, NodeProps } from '@xyflow/react'

import { NodeShell } from '../shared/NodeShell'
import type { CatalogNodeData } from '../../catalogTypes'

const TYPE_KEY = 'core/start'

// core/start declares no control-in — it's the entry point, so there's
// nothing upstream of it. The configured trigger is visible via the
// settings modal rather than on the node face — see NodeShell's compact,
// shape-only layout.
export function StartNode({ id, data }: NodeProps<Node<CatalogNodeData>>) {
  return <NodeShell id={id} data={data} typeKey={TYPE_KEY} />
}
