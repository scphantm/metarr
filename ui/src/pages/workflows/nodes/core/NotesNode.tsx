import { useReactFlow, type Node, type NodeProps } from '@xyflow/react'

import type { CatalogNodeData } from '../../catalogTypes'

/*
 * core/note declares zero ports in the catalog — notes are stripped before
 * compilation and excluded from every validation pass — so this is the one
 * node with no handles at all, and skips NodeShell entirely: no name, no
 * edit modal, just the note text itself, directly editable in place.
 *
 * This is a deliberate change from the pre-redesign NotesNode, which had a
 * single stacked target+source "connection" handle (with a duplicate-id bug
 * to boot). That UX no longer matches the catalog: a note is a floating
 * annotation now, not a node other nodes wire through.
 */
export function NotesNode({ id, data }: NodeProps<Node<CatalogNodeData>>) {
  const { updateNodeData } = useReactFlow()
  const notes = typeof data.settings.notes === 'string' ? data.settings.notes : ''

  return (
    <div className="min-w-[160px] max-w-[240px] rounded border border-edge-strong/40 border-l-4 border-l-orange bg-surface px-3 py-2 shadow-sm">
      <textarea
        value={notes}
        onChange={(event) => updateNodeData(id, { settings: { ...data.settings, notes: event.target.value } })}
        placeholder="Note…"
        disabled={data.readOnly}
        rows={3}
        className="nodrag w-full resize-none rounded border border-transparent bg-transparent text-xs text-ink-strong placeholder:text-ink-muted focus:border-edge-strong/40 focus:bg-canvas focus:outline-none disabled:opacity-60"
      />
    </div>
  )
}
