import { useState } from 'react'

import { Button } from '../components/Card'
import type { ChatToolCall } from '../api/types'
import { useActivePageContext } from '../pagecontext/PageContextRegistry'

const PROPOSE_WORKFLOW_EDIT = 'propose_workflow_edit'

type ProposeWorkflowEditArgs = { graph: unknown; summary: string }

/*
 * Renders the workflow page's propose_workflow_edit tool call — the only
 * tool any page currently contributes (see
 * pagecontext.WorkflowAssembler.Tools on the Go side, gated to the
 * workflow page alone). Approve pushes the proposed graph onto the live
 * canvas via the active page's applyToolResult (registered by
 * WorkflowEditorPage) — never to Mongo directly. The user still presses
 * Save Workflow to persist it, which is what already, unconditionally,
 * creates a new version — no separate "apply" endpoint exists.
 */
export function ProposedEditCard({ toolCall }: { toolCall: ChatToolCall }) {
  const activePageContext = useActivePageContext()
  const [status, setStatus] = useState<'pending' | 'applied' | 'rejected'>('pending')

  if (toolCall.name !== PROPOSE_WORKFLOW_EDIT) return null

  const args = toolCall.arguments as ProposeWorkflowEditArgs
  // The tool is workflow-specific — it's only ever actionable while a
  // workflow page happens to be the one currently mounted. Navigate away
  // (or never had one open) and there's nothing left to apply it to.
  const canApply = activePageContext?.pageKey === 'workflow'

  function approve() {
    activePageContext?.applyToolResult?.(PROPOSE_WORKFLOW_EDIT, toolCall.arguments)
    setStatus('applied')
  }

  return (
    <div className="max-w-[85%] self-start rounded-lg border border-edge bg-surface-hover px-3 py-2 text-sm">
      <p className="text-ink-strong">{args.summary}</p>

      {status === 'pending' ? (
        canApply ? (
          <div className="mt-2 flex gap-2">
            <Button variant="primary" onClick={approve}>
              Approve
            </Button>
            <Button variant="default" onClick={() => setStatus('rejected')}>
              Reject
            </Button>
          </div>
        ) : (
          <p className="mt-2 text-xs text-ink-muted">Reopen this workflow to apply the change.</p>
        )
      ) : status === 'applied' ? (
        <p className="mt-2 text-xs text-green">Applied to the canvas — press Save Workflow to keep it.</p>
      ) : (
        <p className="mt-2 text-xs text-ink-muted">Rejected.</p>
      )}
    </div>
  )
}
