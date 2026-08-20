import { createContext, useContext, useState, type ReactNode } from 'react'

import type { NodeCatalogEntry } from './nodeCatalogTypes'

/*
 * Shares the catalog entry currently being dragged from the palette between
 * the palette and the canvas, using plain native HTML5 drag-and-drop —
 * matching React Flow's own documented drag-and-drop example exactly (no
 * neodrag: canvas node dragging and edge connection are React Flow's own
 * built-in systems regardless, and neodrag isn't documented for either).
 *
 * The whole catalog entry is carried, not just a type string like the
 * official example — the canvas needs the template's other fields
 * (pluginName, version, inputsDB, ...) to build the dropped node's data.
 */

type DnDContextValue = {
  draggedTemplate: NodeCatalogEntry | null
  setDraggedTemplate: (template: NodeCatalogEntry | null) => void
}

const DnDContext = createContext<DnDContextValue | null>(null)

export function DnDProvider({ children }: { children: ReactNode }) {
  const [draggedTemplate, setDraggedTemplate] = useState<NodeCatalogEntry | null>(null)
  return (
    <DnDContext.Provider value={{ draggedTemplate, setDraggedTemplate }}>
      {children}
    </DnDContext.Provider>
  )
}

export function useDnD() {
  const context = useContext(DnDContext)
  if (!context) {
    throw new Error('useDnD must be used within a DnDProvider')
  }
  return context
}
