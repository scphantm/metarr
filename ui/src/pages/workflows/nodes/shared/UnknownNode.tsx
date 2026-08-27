import type { NodeProps, Node } from '@xyflow/react'

import type { UnknownNodeData } from '../../catalogTypes'
import './UnknownNode.css'

/*
 * Catalog drift, design.md §9: a saved node whose type isn't in the loaded
 * catalog (removed, renamed, or a build that's out of sync with the file it
 * was saved against). The node is never dropped — its id, position,
 * settings and label all still round-trip through graphAdapter.ts — it's
 * just rendered as visibly broken instead of guessed at. No handles: with no
 * catalog entry there's no port list to draw them from, and any edges still
 * referencing this node get flagged by POST /api/workflows/validate.
 */
export function UnknownNode({ data }: NodeProps<Node<UnknownNodeData>>) {
  return (
    <div className="unknown-node">
      <div className="unknown-node-title">Unknown node type</div>
      <div className="unknown-node-type">{data.catalogType}</div>
      {data.label ? <div className="unknown-node-label">{data.label}</div> : null}
    </div>
  )
}
