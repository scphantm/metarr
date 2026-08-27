import { useMemo, useState } from 'react'
import { Collapse, Input, Typography } from 'antd'

import { useWorkflowCatalog } from '../../api/queries'
import { useDnD } from './DnDContext'
import type { NodeType } from './catalogTypes'
import './NodePalette.css'

// Two accordion levels: category (outer) then subcategory (inner). Neither
// is required on a catalog entry yet — subcategory is only assigned for a
// handful of entries so far (see catalog.json), category fewer still — so
// both fall back to an 'uncategorized' bucket rather than dropping an
// entry from the palette while the catalog is mid-migration.
function groupByCategoryAndSubcategory(entries: NodeType[]) {
  const groups = new Map<string, Map<string, NodeType[]>>()
  for (const entry of entries) {
    const category = entry.category ?? ''
    const subcategory = entry.subcategory ?? ''
    let subgroups = groups.get(category)
    if (!subgroups) {
      subgroups = new Map()
      groups.set(category, subgroups)
    }
    const group = subgroups.get(subcategory) ?? []
    group.push(entry)
    subgroups.set(subcategory, group)
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
  const groups = groupByCategoryAndSubcategory(filtered)

  return (
    <div className="node-palette">
      <Typography.Text type="secondary" className="node-palette-heading">
        Nodes
      </Typography.Text>

      <Input
        value={filter}
        onChange={(event) => setFilter(event.target.value)}
        placeholder="Filter…"
        size="small"
      />

      {isLoading ? (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          Loading catalog…
        </Typography.Text>
      ) : null}
      {isError ? (
        <Typography.Text type="danger" style={{ fontSize: 12 }}>
          Failed to load the node catalog.
        </Typography.Text>
      ) : null}

      <Collapse
        ghost
        size="small"
        className="node-palette-categories"
        items={[...groups.entries()].map(([category, subgroups]) => ({
          key: category || 'uncategorized',
          label: category || 'Uncategorized',
          children: (
            <Collapse
              ghost
              size="small"
              className="node-palette-subcategories"
              items={[...subgroups.entries()].map(([subcategory, groupEntries]) => ({
                key: subcategory || 'uncategorized',
                label: subcategory || 'Uncategorized',
                children: (
                  <div className="node-palette-entries">
                    {groupEntries.map((entry) => (
                      <div
                        key={entry.id}
                        draggable
                        onDragStart={(event) => {
                          setDraggedTemplate({ id: entry.id })
                          event.dataTransfer.setData('application/json', JSON.stringify({ id: entry.id }))
                          event.dataTransfer.effectAllowed = 'move'
                        }}
                        onDragEnd={() => setDraggedTemplate(null)}
                        title={entry.description}
                        className="node-palette-entry"
                      >
                        <div className="node-palette-entry-name">{entry.name}</div>
                        <div className="node-palette-entry-type">{entry.type}</div>
                      </div>
                    ))}
                  </div>
                ),
              }))}
            />
          ),
        }))}
      />

      <section className="node-palette-legend">
        <Typography.Text type="secondary" className="node-palette-legend-heading">
          Color legend
        </Typography.Text>
        <div className="node-palette-legend-rows">
          <LegendRow colorVar="var(--color-cyan)" label="Input / output / control flow / notes" />
          <LegendRow colorVar="var(--color-red)" label="Errors" />
          <LegendRow colorVar="var(--color-yellow)" label="Decisions" />
          <Typography.Text type="secondary" className="node-palette-legend-subheading">
            Processes
          </Typography.Text>
          <LegendRow colorVar="var(--color-blue)" label="Neutral (e.g. list)" />
          <LegendRow colorVar="var(--color-green)" label="Creation (e.g. copy)" />
          <LegendRow colorVar="var(--color-magenta)" label="Destructive (move, delete, update)" />
          <LegendRow colorVar="var(--color-violet)" label="Create + destroy (complex)" />
        </div>
      </section>
    </div>
  )
}

function LegendRow({ colorVar, label }: { colorVar: string; label: string }) {
  return (
    <div className="node-palette-legend-row">
      <span className="node-palette-legend-dot" style={{ backgroundColor: colorVar }} />
      <span>{label}</span>
    </div>
  )
}
