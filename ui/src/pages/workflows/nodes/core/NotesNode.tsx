import { useReactFlow, type Node, type NodeProps } from '@xyflow/react'

import type { CatalogNodeData } from '../../editorNodeData'
import './NotesNode.css'

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
  const notes =
    typeof data.settings.notes === 'string' ? data.settings.notes : ''

  return (
    <div className="notes-node">
      <textarea
        value={notes}
        onChange={(event) =>
          updateNodeData(id, {
            settings: { ...data.settings, notes: event.target.value },
          })
        }
        placeholder="Note…"
        disabled={data.readOnly}
        rows={3}
        className="nodrag notes-node-textarea"
      />
    </div>
  )
}
