/*
 * Hand-written mirrors of the Go workflow contract's JSON shapes, field for
 * field against their `json:` tags — internal/shared/workflow/{catalog,graph,
 * types,handler}.go — and internal/server/workflow/validate/validate.go for
 * the diagnostics shape. Graph node/edge types are prefixed `Graph*` because
 * React Flow already owns the bare `Node`/`Edge` names.
 *
 * This file has no logic of its own — see graphAdapter.ts for the canonical
 * graph <-> React Flow boundary and connectionRules.ts for the type-lattice
 * rules ported from types.go.
 */

// ---- types.go ----------------------------------------------------------

// A dotted-prefix hierarchy ("path.dir" is a subtype of "path", "agent.slug"
// is a subtype of "agent"), plus the generic `list<T>` constructor. See
// connectionRules.ts for the subtyping/coercion logic itself.
export type Type = string

export type Transform = {
  name: string
  from: Type
  to: Type
  ambiguous?: boolean
  summary?: string
  implies_iteration?: boolean
}

// ---- catalog.go ---------------------------------------------------------

export type NodeKind =
  | ''
  | 'start'
  | 'end'
  | 'fail'
  | 'source'
  | 'branch'
  | 'forEach'
  | 'collect'
  | 'parallel'
  | 'join'
  | 'break'
  | 'note'

export type Effects = 'read' | 'write' | 'destructive'

export type ControlPorts = {
  in: string[]
  out: string[]
  error?: boolean
}

export type Socket = {
  name: string
  label?: string
  type: Type
  required?: boolean
  description?: string
}

export type Setting = {
  name: string
  label?: string
  type: Type
  default?: unknown
  ui?: Record<string, unknown>
  description?: string
}

export type RetrySpec = {
  attempts?: number
  backoff?: string
}

export type ExecSpec = {
  runsOn?: 'server' | 'agent' | ''
  agentSelector?: string
  timeout?: string
  cancellable?: boolean
  effects: Effects
  retry?: RetrySpec
}

export type NodeType = {
  id: string
  type: string
  name: string
  // The palette's two accordion levels — category is the outer group (e.g.
  // "workflow"), subcategory the inner one (e.g. "start"). Presentation
  // only, same as before this split. subcategory isn't assigned catalog-wide
  // yet; category is absent on most entries until it is — see
  // NodePalette.tsx's grouping fallback.
  category?: string
  subcategory?: string
  kind?: NodeKind
  description?: string
  control: ControlPorts
  dataIn?: Socket[]
  dataOut?: Socket[]
  settings?: Setting[]
  exec: ExecSpec
}

export type CatalogResponse = {
  node_types: NodeType[]
  transforms: Transform[]
  schema_version: number
}

// ---- graph.go -------------------------------------------------------------

// The current stored graph format. A document without a matching
// schema_version predates the control/data-edge redesign and is opened
// read-only rather than guessed at — see WorkflowEditorPage.
export const SchemaVersion = 1

export type GraphPosition = {
  x: number
  y: number
}

export type GraphNode = {
  id: string
  type: string
  // The exact catalog entry this instance was placed from — several entries
  // may share `type` (variations of one plugin, e.g. two core/start entries
  // with different dataOut shapes), so this is what resolves it
  // unambiguously. Absent on graphs saved before catalog entries carried an
  // id; those fall back to an arbitrary match by type.
  catalogId?: string
  position: GraphPosition
  settings?: Record<string, unknown>
  promoted?: string[]
  label?: string
  // Per-instance color overrides — independent parameters, each one of the
  // 8 Solarized accent token names (see nodes/shared/nodeVisual.ts).
  // Unknown to the Go backend's typed Node struct, which round-trips them
  // via its Extra field rather than dropping them — see
  // internal/shared/workflow/graph.go.
  shapeColor?: string
  borderColor?: string
}

export type EdgeKind = 'control' | 'data'

export type Endpoint = {
  node: string
  port: string
}

export type GraphEdge = {
  id: string
  kind: EdgeKind
  from: Endpoint
  to: Endpoint
  transform?: string
  // Per-edge configuration, e.g. { recursive: true } on a data edge
  // delivering a path — opened by double-clicking the edge in the editor.
  // No catalog schema, unlike a node's settings (see workflow.Edge.Settings).
  settings?: Record<string, unknown>
}

export type Graph = {
  schema_version: number
  nodes: GraphNode[]
  edges: GraphEdge[]
  viewport?: Record<string, unknown>
}

// ---- validate.go ----------------------------------------------------------

export type Severity = 'error' | 'warning'

export type Diagnostic = {
  severity: Severity
  code: string
  message: string
  node_ids?: string[]
  edge_ids?: string[]
  witness_path?: string[]
}

export type ValidateResponse = {
  diagnostics: Diagnostic[]
  runnable: boolean
}

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
