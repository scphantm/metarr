import { useMemo, useState } from 'react'

import { useWorkflowCatalog } from '../../api/queries'
import { useDnD } from './DnDContext'
import type { NodeType } from './catalogTypes'

function groupByCategory(entries: NodeType[]) {
  const groups = new Map<string, NodeType[]>()
  for (const entry of entries) {
    const category = entry.category ?? ''
    const group = groups.get(category) ?? []
    group.push(entry)
    groups.set(category, group)
  }
  return groups
}

export function NodePalette() {
  const { setDraggedTemplate } = useDnD()
  const { data: catalog, isLoading, isError } = useWorkflowCatalog()
  const [filter, setFilter] = useState('')

  const filtered = useMemo(() => {
    const entries = catalog?.node_types ?? []
    const query = filter.trim().toLowerCase()
    if (!query) return entries
    return entries.filter(
      (entry) => entry.name.toLowerCase().includes(query) || entry.type.toLowerCase().includes(query),
    )
  }, [catalog, filter])
  const groups = groupByCategory(filtered)

  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto border-r border-edge bg-surface p-3">
      <h2 className="px-1 text-xs font-semibold tracking-wide text-ink-muted uppercase">Nodes</h2>

      <input
        value={filter}
        onChange={(event) => setFilter(event.target.value)}
        placeholder="Filter…"
        className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-xs text-ink-strong focus:border-blue"
      />

      {isLoading ? <p className="px-1 text-xs text-ink-muted">Loading catalog…</p> : null}
      {isError ? <p className="px-1 text-xs text-red">Failed to load the node catalog.</p> : null}

      <div className="flex flex-col gap-4">
        {[...groups.entries()].map(([category, groupEntries]) => (
          <section key={category || 'uncategorized'}>
            <h3 className="mb-1.5 px-1 text-[11px] tracking-wide text-ink-muted uppercase">
              {category || 'Uncategorized'}
            </h3>
            <div className="flex flex-col gap-1.5">
              {groupEntries.map((entry) => (
                <div
                  key={entry.type}
                  draggable
                  onDragStart={(event) => {
                    setDraggedTemplate({ type: entry.type, typeVersion: entry.typeVersion })
                    event.dataTransfer.setData(
                      'application/json',
                      JSON.stringify({ type: entry.type, typeVersion: entry.typeVersion }),
                    )
                    event.dataTransfer.effectAllowed = 'move'
                  }}
                  onDragEnd={() => setDraggedTemplate(null)}
                  title={entry.description}
                  className="cursor-grab rounded border border-edge-strong/40 bg-canvas px-2.5 py-2 text-sm text-ink-strong transition-colors hover:border-blue active:cursor-grabbing"
                >
                  <div className="font-medium">{entry.name}</div>
                  <div className="font-mono text-xs text-ink-muted">{entry.type}</div>
                </div>
              ))}
            </div>
          </section>
        ))}
      </div>

      <section className="border-t border-edge-strong/40 pt-3">
        <h3 className="mb-1.5 px-1 text-[11px] tracking-wide text-ink-muted uppercase">Color legend</h3>
        <div className="flex flex-col gap-1 px-1 text-xs text-ink-strong">
          <LegendRow color="bg-cyan" label="Input / output / control flow / notes" />
          <LegendRow color="bg-red" label="Errors" />
          <LegendRow color="bg-yellow" label="Decisions" />
          <p className="mt-1.5 text-[11px] tracking-wide text-ink-muted uppercase">Processes</p>
          <LegendRow color="bg-blue" label="Neutral (e.g. list)" />
          <LegendRow color="bg-green" label="Creation (e.g. copy)" />
          <LegendRow color="bg-magenta" label="Destructive (move, delete, update)" />
          <LegendRow color="bg-violet" label="Create + destroy (complex)" />
        </div>
      </section>
    </div>
  )
}

function LegendRow({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex items-center gap-2">
      <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${color}`} />
      <span>{label}</span>
    </div>
  )
}
