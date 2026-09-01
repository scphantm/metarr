/*
 * Editor-only data carried on a React Flow node.
 *
 * None of this crosses a language boundary, so none of it is generated: it is
 * what a canvas node holds in its `data` slot while being edited, re-resolved
 * live from the fetched catalog and never persisted verbatim. The stored
 * graph is the generated WorkflowGraph message (../../gen/metarr/v1/
 * workflow_graph_pb); graphAdapter.ts is the one place the two are
 * translated, and connectionRules.ts holds the type-lattice rules ported
 * from types.go.
 */

// The current stored graph format. A workflow whose graph reports a different
// schema_version predates the control/data-edge redesign and is opened
// read-only rather than guessed at — see WorkflowEditorPage. Mirrors
// workflow.SchemaVersion on the Go side.
export const SchemaVersion = 1;

// What every catalog-driven node's React Flow `data` holds. Deliberately
// thin: the port/socket/label metadata is never duplicated here — it's
// re-resolved live from the fetched catalog by node type (see
// nodes/shared/NodeShell.tsx), so the catalog stays the single source of
// truth even after it's been re-fetched or hot-reloaded.
export type CatalogNodeData = {
  settings: Record<string, unknown>;
  promoted: string[];
  label?: string;
  // The exact catalog entry this instance was placed from — WorkflowGraphNode
  // .catalogId. Present on every catalog-driven node (registered or unknown),
  // not just unknown ones, because React Flow's own `type` field can't
  // disambiguate several entries sharing one type.
  catalogId?: string;
  // Per-instance color overrides. The graph message has no field for them, so
  // graphAdapter.ts moves them into and out of WorkflowGraphNode.extra. Each
  // falls back independently to the type-computed color
  // (nodes/shared/nodeVisual.ts) when unset.
  shapeColor?: string;
  borderColor?: string;
  // Live notification signal, not authored graph content — never written back
  // to the graph message, so a saved workflow can't carry stale
  // test-animation state. One accent token name per quadrant (top-left,
  // top-right, bottom-left, bottom-right); undefined slots stay invisible.
  // Driven today by WorkflowCanvas's "Test animate" checkbox, later by real
  // run status.
  quadrantColors?: (string | undefined)[];
  // Set while a historic version is being viewed, or the node's own type
  // isn't in the loaded catalog — see WorkflowCanvas's displayNodes.
  readOnly?: boolean;
};

// Carried by nodes rendered as UnknownNode: either the catalog has no entry
// for this type at all (design.md §9, catalog drift), or the registry has no
// component for it (should not happen if the registry is generated from the
// catalog, but guards a stale build). React Flow's own `type` field is
// overwritten to UNKNOWN_NODE_TYPE for these, so the real type has to be
// stashed here instead.
export type UnknownNodeData = CatalogNodeData & {
  catalogType: string;
};
