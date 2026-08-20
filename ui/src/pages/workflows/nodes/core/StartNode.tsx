import type { Node, NodeProps } from '@xyflow/react'

import { NodeShell } from '../shared/NodeShell'
import type { CatalogNodeData } from '../../catalogTypes'

const TYPE_KEY = 'core/start'

// core/start declares no control-in — it's the entry point, so there's
// nothing upstream of it — and its own configured trigger is worth seeing
// at a glance rather than only in the settings modal.
export function StartNode({ id, data }: NodeProps<Node<CatalogNodeData>>) {
  const trigger = typeof data.settings.trigger === 'string' ? data.settings.trigger : 'manual'
  return (
    <NodeShell id={id} data={data} typeKey={TYPE_KEY}>
      <div className="mt-1 text-[11px] text-ink-muted">
        Trigger: <span className="font-mono text-ink-strong">{trigger}</span>
      </div>
    </NodeShell>
  )
}
