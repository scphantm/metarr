import type { NodeProps, Node } from '@xyflow/react'

import type { UnknownNodeData } from '../../catalogTypes'

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
    <div className="min-w-[160px] rounded border-2 border-dashed border-red/60 bg-surface px-3 py-2 shadow-sm">
      <div className="text-xs font-semibold text-red">Unknown node type</div>
      <div className="mt-0.5 font-mono text-[11px] text-ink-muted">{data.catalogType}</div>
      {data.label ? <div className="mt-1 text-xs text-ink-strong">{data.label}</div> : null}
    </div>
  )
}
