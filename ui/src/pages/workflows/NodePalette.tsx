import nodeCatalog from './nodeCatalog.json'
import type { NodeCatalogEntry } from './nodeCatalogTypes'
import { useDnD } from './DnDContext'

const catalog = nodeCatalog as NodeCatalogEntry[]

function groupByCategory(entries: NodeCatalogEntry[]) {
  const groups = new Map<string, NodeCatalogEntry[]>()
  for (const entry of entries) {
    const group = groups.get(entry.category) ?? []
    group.push(entry)
    groups.set(entry.category, group)
  }
  return groups
}

export function NodePalette() {
  const { setDraggedTemplate } = useDnD()
  const groups = groupByCategory(catalog)

  return (
    <div className="flex h-full flex-col gap-4 overflow-y-auto border-r border-edge bg-surface p-3">
      <h2 className="px-1 text-xs font-semibold tracking-wide text-ink-muted uppercase">
        Nodes
      </h2>

      {[...groups.entries()].map(([category, entries]) => (
        <section key={category}>
          <h3 className="mb-1.5 px-1 text-[11px] tracking-wide text-ink-muted uppercase">
            {category}
          </h3>
          <div className="flex flex-col gap-1.5">
            {entries.map((entry, index) => (
              <div
                key={`${entry.id}-${index}`}
                draggable
                onDragStart={(event) => {
                  setDraggedTemplate(entry)
                  event.dataTransfer.setData('application/json', JSON.stringify(entry))
                  event.dataTransfer.effectAllowed = 'move'
                }}
                onDragEnd={() => setDraggedTemplate(null)}
                className="cursor-grab rounded border border-edge-strong/40 bg-canvas px-2.5 py-2 text-sm text-ink-strong transition-colors hover:border-blue active:cursor-grabbing"
              >
                <div className="font-medium">{entry.name}</div>
                <div className="font-mono text-xs text-ink-muted">{entry.pluginName}</div>
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}
