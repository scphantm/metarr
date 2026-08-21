import { useState } from 'react'
import { createPortal } from 'react-dom'

import { Button } from '../../../components/Card'

/*
 * The edit form for one data edge's settings, opened by double-clicking a
 * path-typed edge (see DataEdge.tsx). Same portal-to-document.body pattern
 * as NodeSettingsEditor.tsx, for the same reason: the edge lives inside
 * React Flow's zoomed/panned canvas transform.
 *
 * Unlike a node's settings, an edge has no catalog-declared Setting[] to
 * drive a generic form from — there is no per-type edge schema
 * (workflow.Edge.Settings has no catalog counterpart). Recursive is
 * currently the only edge setting that exists, hardcoded here rather than
 * built from a list of one; generalize this into a catalog-driven form only
 * once a second edge setting actually needs one.
 */
export function EdgeSettingsEditor({
  recursive,
  onSave,
  onCancel,
}: {
  recursive: boolean
  onSave: (next: { recursive: boolean }) => void
  onCancel: () => void
}) {
  const [recursiveDraft, setRecursiveDraft] = useState(recursive)

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onCancel}>
      <div
        className="w-full max-w-md rounded-lg border border-edge bg-surface p-5 shadow-lg"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 className="text-sm font-semibold text-ink-strong">Path connection settings</h2>

        <div className="mt-4 flex flex-col gap-3 border-t border-edge/60 pt-4">
          <div className="flex flex-col gap-1">
            <label className="flex items-center gap-2 text-xs text-ink-muted" htmlFor="edge-setting-recursive">
              <input
                id="edge-setting-recursive"
                type="checkbox"
                checked={recursiveDraft}
                onChange={(event) => setRecursiveDraft(event.target.checked)}
                className="h-4 w-4"
              />
              Recursive
            </label>
            <p className="text-[11px] text-ink-muted">
              Include subdirectories when this connection&rsquo;s path destination is used.
            </p>
          </div>
        </div>

        <div className="mt-5 flex justify-end gap-2">
          <Button variant="default" onClick={onCancel}>
            Cancel
          </Button>
          <Button variant="primary" onClick={() => onSave({ recursive: recursiveDraft })}>
            Save
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
