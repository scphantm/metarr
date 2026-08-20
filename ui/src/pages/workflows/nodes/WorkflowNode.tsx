import { Handle, Position, type Node, type NodeProps } from '@xyflow/react'

/*
 * One generic node renderer for every catalog category, not one component
 * per category — the catalog's categories are expected to keep changing as
 * custom nodes are designed, so hard-coding a component per category would
 * fight that. Only the accent color and which handles are shown vary by
 * category; everything else about the card is the same.
 */

export type WorkflowNodeData = {
  name: string
  sourceRepo: string
  pluginName: string
  version: string
  category: string
  inputsDB?: Record<string, unknown>
}

export type WorkflowNodeType = Node<WorkflowNodeData, 'catalogNode'>

const categoryAccent: Record<string, string> = {
  input: 'border-l-cyan',
  output: 'border-l-violet',
  check: 'border-l-yellow',
}

function accentFor(category: string) {
  return categoryAccent[category] ?? 'border-l-blue'
}

export function WorkflowNode({ data }: NodeProps<WorkflowNodeType>) {
  const showSource = data.category !== 'output'
  const showTarget = data.category !== 'input'
  const inputEntries = data.inputsDB ? Object.entries(data.inputsDB) : []

  return (
    <div
      className={`min-w-[180px] rounded border border-edge-strong/40 border-l-4 bg-surface px-3 py-2 shadow-sm ${accentFor(data.category)}`}
    >
      {showTarget ? (
        <Handle type="target" position={Position.Left} className="!bg-ink-muted" />
      ) : null}

      <div className="text-sm font-semibold text-ink-strong">{data.name}</div>
      <div className="mt-0.5 font-mono text-xs text-ink-muted">
        {data.pluginName}@{data.version}
      </div>
      <div className="text-[10px] tracking-wide text-ink-muted uppercase">{data.sourceRepo}</div>

      {inputEntries.length > 0 ? (
        <dl className="mt-2 space-y-0.5 border-t border-edge/60 pt-1.5">
          {inputEntries.map(([key, value]) => (
            <div key={key} className="flex justify-between gap-2 text-[11px]">
              <dt className="text-ink-muted">{key}</dt>
              <dd className="truncate text-ink">{String(value)}</dd>
            </div>
          ))}
        </dl>
      ) : null}

      {showSource ? (
        <Handle type="source" position={Position.Right} className="!bg-ink-muted" />
      ) : null}
    </div>
  )
}
