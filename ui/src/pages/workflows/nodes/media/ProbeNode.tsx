import type { Node, NodeProps } from '@xyflow/react'

import { NodeShell } from '../shared/NodeShell'
import type { CatalogNodeData } from '../../catalogTypes'

const TYPE_KEY = 'media/probe'

export function ProbeNode({ id, data }: NodeProps<Node<CatalogNodeData>>) {
  return <NodeShell id={id} data={data} typeKey={TYPE_KEY} />
}
