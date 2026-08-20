import { useState, type ReactNode } from 'react'
import { Handle, Position, useReactFlow } from '@xyflow/react'

import { accentClassForCategory, dataHandleClass } from '../../../../lib/typeColors'
import { controlHandleId } from '../../connectionRules'
import { useCatalogEntry } from '../../useCatalogEntry'
import { nodeTypeKey, type CatalogNodeData } from '../../catalogTypes'
import { EditIcon } from './EditIcon'
import { NodeSettingsEditor } from './NodeSettingsEditor'
import { errorHandleTitle, handleOffset, useNodeHandles, type ArrangedHandles } from './useNodeHandles'

/*
 * The common chrome every catalog-driven node is built on: accent border,
 * handles arranged per useNodeHandles, name/label, an effects badge, and the
 * settings-edit button + modal. Ports/sockets/labels are always resolved
 * live from the fetched catalog by `typeKey` — never hardcoded per node file
 * — so the ~30 files under nodes/{core,fs,media,nfo,string}/ stay thin
 * wrappers rather than 30 copies of this logic. A node whose own body needs
 * to diverge (Notes, CheckFlowVariable, Start) skips this component and
 * renders itself directly instead.
 */

// Square-ish and a distinct ink tone from data handles' type coloring, so
// the two port kinds read as visually different before a drag even starts.
const controlHandleClass = '!rounded-[3px] !h-2.5 !w-2.5 !border-ink-strong !bg-ink-strong'

export function NodeShell({
  id,
  data,
  typeKey,
  children,
  handles: handlesOverride,
}: {
  id: string
  data: CatalogNodeData
  typeKey: string
  // Extra content rendered between the header and the settings button —
  // used by the handful of node types that show a live value inline
  // (Start's trigger, Trickplay's dimensions) without needing a fully
  // custom body.
  children?: ReactNode
  // Pre-arranged handles to render instead of the catalog's full set — used
  // by node types whose visible port count depends on their own settings
  // rather than being fixed by the catalog alone (Parallel, Join; see
  // nodes/shared/branchPorts.ts).
  handles?: ArrangedHandles
}) {
  const { updateNodeData } = useReactFlow()
  const [editing, setEditing] = useState(false)
  const nodeType = useCatalogEntry(typeKey)
  const catalogHandles = useNodeHandles(nodeType)
  const handles = handlesOverride ?? catalogHandles

  if (!nodeType) {
    // The catalog hasn't loaded yet, or (should not happen if
    // nodes/registry.ts is generated from the same catalog) this type has
    // vanished from it since the page loaded.
    return (
      <div className="min-w-[140px] rounded border border-dashed border-edge-strong/40 bg-surface px-3 py-3 text-xs text-ink-muted shadow-sm">
        {typeKey}
      </div>
    )
  }

  const label = data.label ?? nodeType.name
  const settings = nodeType.settings ?? []
  const canEdit = settings.length > 0 && !data.readOnly
  const accentClass = accentClassForCategory(nodeType.category)

  return (
    <div
      className={`min-w-[150px] rounded border border-edge-strong/40 border-l-4 bg-surface px-3 py-3 shadow-sm ${accentClass}`}
    >
      {handles.top.map((handle, index) => (
        <Handle
          key={handle.id}
          id={handle.id}
          type="target"
          position={Position.Top}
          style={{ left: handleOffset(index, handles.top.length) }}
          className={handle.kind === 'control' ? controlHandleClass : dataHandleClass(handle.type ?? 'any')}
          title={handle.title}
        />
      ))}
      {handles.bottom.map((handle, index) => (
        <Handle
          key={handle.id}
          id={handle.id}
          type="source"
          position={Position.Bottom}
          style={{ left: handleOffset(index, handles.bottom.length) }}
          className={handle.kind === 'control' ? controlHandleClass : dataHandleClass(handle.type ?? 'any')}
          title={handle.title}
        />
      ))}
      {handles.hasError ? (
        <Handle
          id={controlHandleId('error')}
          type="source"
          position={Position.Right}
          className="!border-red !bg-red"
          title={errorHandleTitle}
        />
      ) : null}

      <div className="flex items-center gap-1.5">
        {canEdit ? (
          <button
            type="button"
            onClick={() => setEditing(true)}
            aria-label={`Edit ${label} settings`}
            className="shrink-0 text-ink-muted transition-colors hover:text-blue"
          >
            <EditIcon />
          </button>
        ) : null}
        <span className="text-sm font-semibold text-ink-strong">{label}</span>
        {nodeType.exec.effects === 'destructive' ? (
          <span
            title="Destructive: deletes or overwrites existing content"
            className="ml-auto h-1.5 w-1.5 shrink-0 rounded-full bg-red"
          />
        ) : null}
      </div>

      {children}

      {editing ? (
        <NodeSettingsEditor
          nodeName={label}
          typeKey={nodeTypeKey(nodeType.type, nodeType.typeVersion)}
          description={nodeType.description}
          settings={settings}
          values={data.settings}
          onSave={(next) => {
            updateNodeData(id, { settings: next })
            setEditing(false)
          }}
          onCancel={() => setEditing(false)}
        />
      ) : null}
    </div>
  )
}
