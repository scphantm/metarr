import { Handle, Position, useReactFlow, type Node, type NodeProps } from '@xyflow/react'

import { dataHandleClass } from '../../../../lib/typeColors'
import { useCatalogEntry } from '../../useCatalogEntry'
import type { CatalogNodeData } from '../../catalogTypes'
import { errorHandleTitle, handleOffset, useNodeHandles } from '../shared/useNodeHandles'

const TYPE_KEY = 'core/checkFlowVariable'

const controlHandleClass = '!rounded-[3px] !h-2.5 !w-2.5 !border-ink-strong !bg-ink-strong'
const operatorOptions = ['==', '!=', 'contains', '>', '<', '>=', '<=']

/*
 * The one control-flow node that keeps its condition on the node face
 * rather than behind the settings modal — the comparison IS the node, the
 * same reasoning the pre-redesign version used. Everything else (handles)
 * still comes straight off the live catalog via useNodeHandles, so this
 * stays in sync if the catalog entry's ports ever change.
 */
export function CheckFlowVariableNode({ id, data }: NodeProps<Node<CatalogNodeData>>) {
  const { updateNodeData } = useReactFlow()
  const nodeType = useCatalogEntry(TYPE_KEY)
  const handles = useNodeHandles(nodeType)

  const data1 = typeof data.settings.data1 === 'string' ? data.settings.data1 : ''
  const operator = typeof data.settings.operator === 'string' ? data.settings.operator : ''
  const data2 = typeof data.settings.data2 === 'string' ? data.settings.data2 : ''

  // The stored operator might not be one of the common ones offered below
  // (a hand-edited catalog default, or a value from before this list
  // existed) — keep it selectable rather than silently swapping it out.
  const operators = operatorOptions.includes(operator) || !operator ? operatorOptions : [operator, ...operatorOptions]

  const fieldClass =
    'w-full rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-xs text-ink-strong focus:border-blue disabled:opacity-60'

  function setSetting(name: string, value: string) {
    updateNodeData(id, { settings: { ...data.settings, [name]: value } })
  }

  return (
    <div className="min-w-[220px] rounded border border-edge-strong/40 border-l-4 border-l-yellow bg-surface px-3 py-3 shadow-sm">
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
          id="c:error"
          type="source"
          position={Position.Right}
          className="!border-red !bg-red"
          title={errorHandleTitle}
        />
      ) : null}

      <div className="text-sm font-semibold text-ink-strong">{data.label ?? nodeType?.name ?? 'Check flow variable'}</div>

      <div className="nodrag mt-2 flex flex-col gap-1.5">
        <input
          value={data1}
          onChange={(event) => setSetting('data1', event.target.value)}
          placeholder="data1"
          disabled={data.readOnly}
          className={fieldClass}
        />
        <select
          value={operator}
          onChange={(event) => setSetting('operator', event.target.value)}
          disabled={data.readOnly}
          className={fieldClass}
        >
          {!operator ? <option value="">operator…</option> : null}
          {operators.map((option) => (
            <option key={option} value={option}>
              {option}
            </option>
          ))}
        </select>
        <input
          value={data2}
          onChange={(event) => setSetting('data2', event.target.value)}
          placeholder="data2"
          disabled={data.readOnly}
          className={fieldClass}
        />
      </div>
    </div>
  )
}
