import { createPortal } from 'react-dom'

import { Button } from '../components/Card'
import type { ContextSentRecord } from '../api/types'

/*
 * Shows exactly what was sent to the model alongside one message — opened
 * from the context icon on that message (see ChatPanel.tsx). Same
 * portal/backdrop pattern as NodeSettingsEditor.tsx: rendered to
 * document.body since the widget it's opened from is itself a fixed-
 * position overlay, and parent owns open/close via conditional render.
 */
export function ContextSentModal({
  record,
  onClose,
}: {
  record: ContextSentRecord
  onClose: () => void
}) {
  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onClose}
    >
      <div
        className="flex max-h-[80vh] w-full max-w-lg flex-col rounded-lg border border-edge bg-surface p-5 shadow-lg"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 className="text-sm font-semibold text-ink-strong">Context sent</h2>
        <p className="mt-1 font-mono text-xs text-ink-muted">{record.page_key}</p>

        <div className="mt-4 flex flex-col gap-3 overflow-y-auto">
          {record.items.map((item) => (
            <details key={item.label} className="rounded border border-edge">
              <summary className="cursor-pointer px-3 py-2 text-sm text-ink-strong">
                {item.label}
                <span className="ml-2 text-xs text-ink-muted">
                  {item.description} — ~{item.token_estimate} tokens
                </span>
              </summary>
              <pre className="overflow-x-auto border-t border-edge bg-canvas px-3 py-2 text-xs whitespace-pre-wrap text-ink">
                {JSON.stringify(item.detail, null, 2)}
              </pre>
            </details>
          ))}
        </div>

        <div className="mt-4 flex justify-end">
          <Button variant="default" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
