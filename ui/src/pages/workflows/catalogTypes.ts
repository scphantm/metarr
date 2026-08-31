/*
 * Editor-only data types for the workflow canvas.
 *
 * The graph model itself — nodes, edges, endpoints, the edge-kind
 * vocabulary — is generated now (WorkflowGraph and friends in
 * ../../gen/metarr/v1/workflow_graph_pb), as is the catalog half and the
 * validation diagnostics (workflow_catalog_pb). See docs/adr/0005. What
 * remains here is what a React Flow node carries in its `data` slot, which
 * has no proto message because it never crosses a language boundary — it is
 * re-resolved live from the fetched catalog and never persisted verbatim.
 *
 * graphAdapter.ts is the one place the generated graph message is translated
 * to and from React Flow's own node/edge shape; connectionRules.ts holds the
 * type-lattice rules ported from types.go.
 */

// The current stored graph format. A document whose graph reports a
// different schema_version predates the control/data-edge redesign and is
// opened read-only rather than guessed at — see WorkflowEditorPage.
export const SchemaVersion = 1

// ---- editor-only data carried on a React Flow node -------------------------

// What every catalog-driven node's React Flow `data` holds. Deliberately
// thin: the port/socket/label metadata is never duplicated here — it's
// re-resolved live from the fetched catalog by node type (see
// nodes/shared/NodeShell.tsx), so the catalog stays the single source of
// truth even after it's been re-fetched or hot-reloaded.
export type CatalogNodeData = {
  settings: Record<string, unknown>
  promoted: string[]
  label?: string
  // The exact catalog entry this instance was placed from — see
  // GraphNode.catalogId above. Present on every catalog-driven node
  // (registered or unknown), not just unknown ones, because React Flow's own
  // `type` field can't disambiguate several entries sharing one type.
  catalogId?: string
  // Per-instance color overrides — see GraphNode.shapeColor/borderColor
  // above. Each falls back independently to the type-computed color
  // (nodes/shared/nodeVisual.ts) when unset.
  shapeColor?: string
  borderColor?: string
  // Live notification signal, not authored graph content — deliberately
  // absent from GraphNode above, so graphAdapter.ts never persists it and a
  // saved workflow can't carry stale test-animation state. One accent
  // token name per quadrant (top-left, top-right, bottom-left,
  // bottom-right); undefined slots stay invisible. Driven today by
  // WorkflowCanvas's "Test animate" checkbox, later by real run status.
  quadrantColors?: (string | undefined)[]
  // Set while a historic version is being viewed, or the node's own type
  // isn't in the loaded catalog — see WorkflowCanvas's displayNodes.
  readOnly?: boolean
}

// Carried by nodes rendered as UnknownNode: either the catalog has no entry
// for this type at all (design.md §9, catalog drift), or the registry has no
// component for it (should not happen if the registry is generated from the
// catalog, but guards a stale build). React Flow's own `type` field is
// overwritten to UNKNOWN_NODE_TYPE for these, so the real type has to be
// stashed here instead.
export type UnknownNodeData = CatalogNodeData & {
  catalogType: string
}
