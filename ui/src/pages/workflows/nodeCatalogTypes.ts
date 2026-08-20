// The shape of ui/src/pages/workflows/nodeCatalog.json — the hand-edited
// catalog of node types offered in the editor's palette. This will keep
// growing fields as custom node types are added, so it stays deliberately
// loose rather than a closed/exhaustive type.

export type NodeCategory = 'input' | 'output' | 'check' | (string & {})

export type NodeCatalogEntry = {
  name: string
  sourceRepo: string
  pluginName: string
  version: string
  category: NodeCategory
  // Informational only, copy-pasted from wherever the catalog entry came
  // from — not unique across entries (see "Input File"/"Output File" in the
  // sample catalog) and never used as a React key or node instance id.
  id: string
  // Also a leftover from the catalog's source, not a real canvas position —
  // always overwritten with the actual drop location when a node is created.
  position: { x: number; y: number }
  inputsDB?: Record<string, unknown>
  [key: string]: unknown
}
