import { forwardRef, useImperativeHandle, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { ReactFlowProvider, type Edge, type Node, type ReactFlowInstance, type Viewport } from '@xyflow/react'

import type { Workflow } from '../../api/types'
import { queryKeys, useSaveWorkflow, useWorkflow, useWorkflowVersion, useWorkflowVersions } from '../../api/queries'
import { Button } from '../../components/Card'
import { DnDProvider } from './DnDContext'
import { NodePalette } from './NodePalette'
import { TagsInput } from './TagsInput'
import { VersionHistory } from './VersionHistory'
import { WorkflowCanvas } from './WorkflowCanvas'
import { clearStashedDraft, readStashedDraft, stashDraft, type StashedDraft } from './draftStorage'

const emptyViewport: Viewport = { x: 0, y: 0, zoom: 1 }
const emptySnapshot: StashedDraft = { name: '', description: '', tags: [], nodes: [], edges: [], viewport: emptyViewport }

function snapshotFromWorkflow(workflow: Workflow): StashedDraft {
  return {
    name: workflow.name,
    description: workflow.description,
    tags: workflow.tags,
    nodes: workflow.nodes as Node[],
    edges: workflow.edges as Edge[],
    viewport: workflow.viewport as Viewport,
  }
}

function draftDiffers(a: StashedDraft, b: StashedDraft) {
  const strip = (s: StashedDraft) => ({ name: s.name, description: s.description, tags: s.tags, nodes: s.nodes, edges: s.edges })
  return JSON.stringify(strip(a)) !== JSON.stringify(strip(b))
}

type EditorHandle = { getSnapshot: () => StashedDraft }

/*
 * Shared by both /workflows/add and /workflows/:id/edit — an id present in
 * the route is the only difference between "add" and "edit". Add-mode
 * transitions into edit-mode in place after the first successful save (see
 * EditorBody's handleSave), since the workflow only gets an id and version
 * history once it has been saved at least once.
 *
 * EditorBody owns the actual editable state (name/description/tags/canvas),
 * initialized once from its `initial` prop via lazy useState — never from an
 * effect reacting to query data. Every point where the displayed content
 * should reset (a workflow finishes loading, a historic version is opened,
 * "Back to editing" is clicked) is a `key` change on EditorBody below, which
 * remounts it fresh with new initial values instead of reactively
 * overwriting in-progress state.
 */
export function WorkflowEditorPage() {
  const { id } = useParams<{ id?: string }>()
  const navigate = useNavigate()

  const workflowQuery = useWorkflow(id ?? '')
  const versionsQuery = useWorkflowVersions(id ?? '')
  const queryClient = useQueryClient()

  const [mode, setMode] = useState<'editing' | 'viewing-version'>('editing')
  const [viewingVersion, setViewingVersion] = useState<number | null>(null)
  // Bumped on "Back to editing" so EditorBody remounts even when mode and
  // viewingVersion both return to their prior values — those alone wouldn't
  // change the key on a second visit to the same old version.
  const [restoreNonce, setRestoreNonce] = useState(0)
  // Set only when "Back to editing" found something stashed; null means
  // "nothing had changed, fall back to workflowQuery.data as normal". A real
  // state value (not a ref) so it can be read during render.
  const [restoredDraft, setRestoredDraft] = useState<StashedDraft | null>(null)

  const viewingVersionQuery = useWorkflowVersion(id ?? '', viewingVersion)

  const editorRef = useRef<EditorHandle>(null)

  // The query cache is the baseline — no separate ref/state mirrors it. Save
  // seeds the cache directly (see handleSaved) so this is fresh immediately,
  // not only after the invalidated query's background refetch resolves.
  const baseline = workflowQuery.data ? snapshotFromWorkflow(workflowQuery.data) : null

  function handleSelectVersion(version: number) {
    if (!id) return
    const current = editorRef.current?.getSnapshot()
    if (current && (!baseline || draftDiffers(current, baseline))) {
      stashDraft(id, current)
    }
    setMode('viewing-version')
    setViewingVersion(version)
  }

  function handleBackToEditing() {
    if (!id) return
    const stashed = readStashedDraft(id)
    if (stashed) {
      clearStashedDraft(id)
    }
    setRestoredDraft(stashed)
    setMode('editing')
    setViewingVersion(null)
    setRestoreNonce((n) => n + 1)
  }

  function handleSaved(saved: Workflow) {
    // Seed the cache immediately rather than waiting on the mutation's
    // invalidation-triggered refetch, so `baseline` above reflects this save
    // on the very next render.
    queryClient.setQueryData(queryKeys.workflow(saved.document_id), saved)
    if (!id) {
      navigate(`/workflows/${saved.document_id}/edit`, { replace: true })
    }
  }

  const readOnly = mode === 'viewing-version'

  let editorKey: string
  let initialSnapshot: StashedDraft
  let ready = true
  if (readOnly) {
    editorKey = `v-${viewingVersion}`
    if (!viewingVersionQuery.data) {
      ready = false
      initialSnapshot = emptySnapshot
    } else {
      initialSnapshot = snapshotFromWorkflow(viewingVersionQuery.data)
    }
  } else if (id) {
    editorKey = `${id}-${restoreNonce}`
    if (restoredDraft) {
      initialSnapshot = restoredDraft
    } else if (!baseline) {
      ready = false
      initialSnapshot = emptySnapshot
    } else {
      initialSnapshot = baseline
    }
  } else {
    editorKey = 'new'
    initialSnapshot = emptySnapshot
  }

  return (
    <div className="flex h-full overflow-hidden">
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        {ready ? (
          <EditorBody
            key={editorKey}
            ref={editorRef}
            documentId={id}
            initial={initialSnapshot}
            readOnly={readOnly}
            viewingVersion={viewingVersion}
            onBackToEditing={handleBackToEditing}
            onSaved={handleSaved}
          />
        ) : (
          <p className="p-6 text-sm text-ink-muted">Loading…</p>
        )}
      </div>

      {id ? (
        <div className="w-48 shrink-0 overflow-y-auto">
          <VersionHistory
            versions={versionsQuery.data ?? []}
            viewingVersion={viewingVersion}
            onSelect={handleSelectVersion}
          />
        </div>
      ) : null}
    </div>
  )
}

const EditorBody = forwardRef<
  EditorHandle,
  {
    documentId: string | undefined
    initial: StashedDraft
    readOnly: boolean
    viewingVersion: number | null
    onBackToEditing: () => void
    onSaved: (saved: Workflow) => void
  }
>(function EditorBody({ documentId, initial, readOnly, viewingVersion, onBackToEditing, onSaved }, ref) {
  const [name, setName] = useState(initial.name)
  const [description, setDescription] = useState(initial.description)
  const [tags, setTags] = useState(initial.tags)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const rfInstanceRef = useRef<ReactFlowInstance | null>(null)
  const saveWorkflow = useSaveWorkflow()

  useImperativeHandle(ref, () => ({
    getSnapshot: () => ({
      name,
      description,
      tags,
      nodes: rfInstanceRef.current?.getNodes() ?? initial.nodes,
      edges: rfInstanceRef.current?.getEdges() ?? initial.edges,
      viewport: rfInstanceRef.current?.getViewport() ?? initial.viewport,
    }),
  }))

  async function handleSave() {
    const instance = rfInstanceRef.current
    if (!instance) return

    setSaving(true)
    setError(null)
    try {
      const graph = instance.toObject()
      const saved = await saveWorkflow.mutateAsync({
        document_id: documentId,
        name: name.trim(),
        description: description.trim(),
        tags,
        nodes: graph.nodes as unknown as Record<string, unknown>[],
        edges: graph.edges as unknown as Record<string, unknown>[],
        viewport: graph.viewport as unknown as Record<string, unknown>,
      })
      onSaved(saved)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setSaving(false)
    }
  }

  const canSave = !readOnly && name.trim() !== '' && description.trim() !== '' && tags.length > 0

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4 border-b border-edge px-6 py-4">
        <div className="flex min-w-0 flex-1 flex-col gap-2">
          <div className="flex flex-wrap items-center gap-3">
            <input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Workflow name"
              disabled={readOnly}
              className="min-w-48 rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm font-semibold text-ink-strong focus:border-blue disabled:opacity-60"
            />
            <input
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder="Description"
              disabled={readOnly}
              className="min-w-64 flex-1 rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm text-ink-strong focus:border-blue disabled:opacity-60"
            />
          </div>
          <div className={readOnly ? 'pointer-events-none opacity-60' : ''}>
            <TagsInput value={tags} onChange={setTags} />
          </div>
          {error ? <p className="text-xs text-red">{error}</p> : null}
        </div>

        <div className="flex items-center gap-2">
          {readOnly ? (
            <>
              <span className="text-xs text-ink-muted">Viewing v{viewingVersion} (read-only)</span>
              <Button variant="default" onClick={onBackToEditing}>
                Back to editing
              </Button>
            </>
          ) : (
            <Button variant="primary" disabled={!canSave || saving} onClick={() => void handleSave()}>
              {saving ? 'Saving…' : 'Save Workflow'}
            </Button>
          )}
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        <DnDProvider>
          <ReactFlowProvider>
            <div className="w-56 shrink-0">
              <NodePalette />
            </div>
            <div className="min-w-0 flex-1">
              <WorkflowCanvas
                initialNodes={initial.nodes}
                initialEdges={initial.edges}
                readOnly={readOnly}
                onInit={(instance) => {
                  rfInstanceRef.current = instance
                }}
              />
            </div>
          </ReactFlowProvider>
        </DnDProvider>
      </div>
    </>
  )
})
