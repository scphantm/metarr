import { createPortal } from 'react-dom'

import { Button } from '../../../components/Card'
import type { Transform, Type } from '../catalogTypes'

/*
 * design.md §4.4, "several candidates": an inline picker with nothing
 * pre-selected. Shared by the new-connection flow (WorkflowCanvas.onConnect,
 * when canConnect() returns more than one candidate, or the sole candidate
 * is marked ambiguous) and by clicking an existing data edge's transform
 * chip to change it (DataEdge). A centered modal rather than a popover
 * pinned to the exact drop point — simpler to get right, and the choice
 * matters far more than its screen position.
 */
export function TransformPicker({
  fromType,
  toType,
  candidates,
  current,
  onPick,
  onClose,
}: {
  fromType: Type
  toType: Type
  candidates: Transform[]
  current?: string
  onPick: (name: string) => void
  onClose: () => void
}) {
  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="w-full max-w-sm rounded-lg border border-edge bg-surface p-4 shadow-lg"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 className="text-sm font-semibold text-ink-strong">Choose a conversion</h2>
        <p className="mt-1 text-xs text-ink-muted">
          <span className="font-mono">{fromType}</span> does not connect directly to{' '}
          <span className="font-mono">{toType}</span>. Pick how to convert it:
        </p>
        <div className="mt-3 flex flex-col gap-1.5">
          {candidates.map((transform) => (
            <button
              key={transform.name}
              type="button"
              onClick={() => onPick(transform.name)}
              className={`rounded border px-2.5 py-1.5 text-left text-xs transition-colors ${
                transform.name === current
                  ? 'border-blue bg-blue/10 text-ink-strong'
                  : 'border-edge-strong/40 text-ink-strong hover:border-blue'
              }`}
            >
              <div className="font-mono font-medium">{transform.name}</div>
              {transform.summary ? <div className="mt-0.5 text-[11px] text-ink-muted">{transform.summary}</div> : null}
            </button>
          ))}
        </div>
        <div className="mt-4 flex justify-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
