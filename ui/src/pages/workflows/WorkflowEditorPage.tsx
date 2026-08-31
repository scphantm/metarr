import { forwardRef, useImperativeHandle, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { ReactFlowProvider, type ReactFlowInstance, type Viewport } from '@xyflow/react'
import { Input, Spin, Typography } from 'antd'

import type { Workflow } from '../../gen/metarr/v1/workflows_pb'
import { queryKeys, useSaveWorkflow, useWorkflow, useWorkflowVersion, useWorkflowVersions } from '../../api/queries'
import { Button } from '../../components/Card'
import { DnDProvider } from './DnDContext'
import { fromRFGraph, toRFGraph } from './graphAdapter'
import { NodePalette } from './NodePalette'
import { registeredTypes } from './nodes/registry'
import { TagsInput } from './TagsInput'
import { VersionHistory } from './VersionHistory'
import { WorkflowCanvas } from './WorkflowCanvas'
import { clearStashedDraft, readStashedDraft, stashDraft, type StashedDraft } from './draftStorage'
import { SchemaVersion } from './catalogTypes'
import './WorkflowEditorPage.css'

const emptyViewport: Viewport = { x: 0, y: 0, zoom: 1 }
const emptySnapshot: StashedDraft = { name: '', description: '', tags: [], nodes: [], edges: [], viewport: emptyViewport }

// A document whose schema_version doesn't match predates the control/data-
// edge redesign (design.md §11) — its nodes/edges are the old React-Flow-
// native shape, not the canonical Graph one, and rendering them through
// toRFGraph would silently produce garbage rather than a clear error. There
// is no migration path, per the project's still-in-development stance: it's
// opened read-only with an explanation instead.
function isCurrentSchema(workflow: Workflow): boolean {
  return (workflow.graph?.schemaVersion ?? 0) === SchemaVersion
}

function snapshotFromWorkflow(workflow: Workflow): StashedDraft {
  const { nodes, edges } = toRFGraph(
    { nodes: workflow.graph?.nodes ?? [], edges: workflow.graph?.edges ?? [] },
    registeredTypes,
  )
  return {
    name: workflow.name,
    description: workflow.description,
    tags: workflow.tags,
    nodes,
    edges,
    viewport: (workflow.graph?.viewport as Viewport | undefined) ?? emptyViewport,
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
    queryClient.setQueryData(queryKeys.workflow(saved.documentId), saved)
    if (!id) {
      navigate(`/workflows/${saved.documentId}/edit`, { replace: true })
    }
  }

  const readOnly = mode === 'viewing-version'

  let editorKey: string
  let initialSnapshot: StashedDraft
  let ready = true
  let outdatedSchema = false
  if (readOnly) {
    editorKey = `v-${viewingVersion}`
    if (!viewingVersionQuery.data) {
      ready = false
      initialSnapshot = emptySnapshot
    } else if (!isCurrentSchema(viewingVersionQuery.data)) {
      outdatedSchema = true
      initialSnapshot = emptySnapshot
    } else {
      initialSnapshot = snapshotFromWorkflow(viewingVersionQuery.data)
    }
  } else if (id) {
    editorKey = `${id}-${restoreNonce}`
    if (restoredDraft) {
      initialSnapshot = restoredDraft
    } else if (!baseline) {
      if (workflowQuery.data && !isCurrentSchema(workflowQuery.data)) {
        outdatedSchema = true
      } else {
        ready = false
      }
      initialSnapshot = emptySnapshot
    } else {
      initialSnapshot = baseline
    }
  } else {
    editorKey = 'new'
    initialSnapshot = emptySnapshot
  }

  return (
    <div className="workflow-editor-page">
      <div className="workflow-editor-main">
        {outdatedSchema ? (
          <div className="workflow-editor-outdated">
            <p>
              This workflow was saved in an older format that predates control/data edges and can&rsquo;t be opened in
              this editor.
            </p>
            <p>Delete it and rebuild it from scratch — this project makes no promise to migrate saved workflows yet.</p>
          </div>
        ) : ready ? (
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
          <div className="workflow-editor-loading">
            <Spin size="small" /> Loading…
          </div>
        )}
      </div>

      {id ? (
        <div className="workflow-editor-history">
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
      const graph = fromRFGraph(instance.getNodes(), instance.getEdges(), instance.getViewport())
      const saved = await saveWorkflow.mutateAsync({
        documentId: documentId ?? '',
        name: name.trim(),
        description: description.trim(),
        tags,
        graph,
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
      <header className="workflow-editor-header">
        <div className="workflow-editor-header-fields">
          <div className="workflow-editor-header-inputs">
            <Input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Workflow name"
              disabled={readOnly}
              className="workflow-editor-name-input"
            />
            <Input
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder="Description"
              disabled={readOnly}
              className="workflow-editor-description-input"
            />
          </div>
          <div className={readOnly ? 'workflow-editor-tags is-read-only' : 'workflow-editor-tags'}>
            <TagsInput value={tags} onChange={setTags} />
          </div>
          {error ? (
            <Typography.Text type="danger" style={{ fontSize: 12 }}>
              {error}
            </Typography.Text>
          ) : null}
        </div>

        <div className="workflow-editor-header-actions">
          {readOnly ? (
            <>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                Viewing v{viewingVersion} (read-only)
              </Typography.Text>
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

      <div className="workflow-editor-body">
        <DnDProvider>
          <ReactFlowProvider>
            <div className="workflow-editor-palette">
              <NodePalette />
            </div>
            <div className="workflow-editor-canvas">
              <WorkflowCanvas
                initialNodes={initial.nodes}
                initialEdges={initial.edges}
                initialViewport={initial.nodes.length > 0 || initial.edges.length > 0 ? initial.viewport : undefined}
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
