import type { Edge as RFEdge, Node as RFNode, Viewport } from '@xyflow/react'

import { controlHandleId, dataHandleId, parseHandleId } from './connectionRules'
import { SchemaVersion, type CatalogNodeData, type Graph, type GraphEdge, type GraphNode } from './catalogTypes'

/*
 * The canonical-graph <-> React Flow boundary. Nothing else in the editor
 * should reach into a GraphNode/GraphEdge's shape directly, or build/read a
 * React Flow node's `type`/`data`/handle ids by hand — this is the one place
 * that translation happens, replacing the old instance.toObject() + `as
 * Node[]` casts that skipped it entirely.
 */

export const UNKNOWN_NODE_TYPE = 'unknownNode'

// registeredTypes is the set of catalog types nodes/registry.ts has a real
// component for. A saved node whose type isn't in it — catalog drift,
// design.md §9 — renders through UnknownNode instead: the node and its
// settings/label are never dropped, just made visible as unrenderable.
export function toRFNode(node: GraphNode, registeredTypes: ReadonlySet<string>): RFNode {
  const data: CatalogNodeData = {
    settings: node.settings ?? {},
    promoted: node.promoted ?? [],
    label: node.label,
    catalogId: node.catalogId,
    shapeColor: node.shapeColor,
    borderColor: node.borderColor,
  }

  if (!registeredTypes.has(node.type)) {
    return {
      id: node.id,
      type: UNKNOWN_NODE_TYPE,
      position: node.position,
      data: { ...data, catalogType: node.type },
    }
  }

  return {
    id: node.id,
    type: node.type,
    position: node.position,
    data,
  }
}

export function fromRFNode(node: RFNode): GraphNode {
  const data = node.data as Partial<CatalogNodeData> & { catalogType?: string }
  const type = data.catalogType ?? node.type ?? ''

  const graphNode: GraphNode = {
    id: node.id,
    type,
    position: { x: node.position.x, y: node.position.y },
  }
  if (data.catalogId) graphNode.catalogId = data.catalogId
  if (data.settings && Object.keys(data.settings).length > 0) graphNode.settings = data.settings
  if (data.promoted && data.promoted.length > 0) graphNode.promoted = data.promoted
  if (data.label) graphNode.label = data.label
  if (data.shapeColor) graphNode.shapeColor = data.shapeColor
  if (data.borderColor) graphNode.borderColor = data.borderColor
  return graphNode
}

export function toRFEdge(edge: GraphEdge): RFEdge {
  const data = edge.transform || edge.settings ? { transform: edge.transform, settings: edge.settings } : undefined
  return {
    id: edge.id,
    type: edge.kind === 'control' ? 'controlEdge' : 'dataEdge',
    source: edge.from.node,
    sourceHandle: edge.kind === 'control' ? controlHandleId(edge.from.port) : dataHandleId(edge.from.port),
    target: edge.to.node,
    targetHandle: edge.kind === 'control' ? controlHandleId(edge.to.port) : dataHandleId(edge.to.port),
    data,
  }
}

export function fromRFEdge(edge: RFEdge): GraphEdge | null {
  const from = parseHandleId(edge.sourceHandle)
  const to = parseHandleId(edge.targetHandle)
  if (!from || !to || from.kind !== to.kind) return null

  const graphEdge: GraphEdge = {
    id: edge.id,
    kind: from.kind === 'control' ? 'control' : 'data',
    from: { node: edge.source, port: from.name },
    to: { node: edge.target, port: to.name },
  }
  const data = edge.data as { transform?: string; settings?: Record<string, unknown> } | undefined
  if (data?.transform) graphEdge.transform = data.transform
  if (data?.settings && Object.keys(data.settings).length > 0) graphEdge.settings = data.settings
  return graphEdge
}

export function toRFGraph(graph: Graph, registeredTypes: ReadonlySet<string>): { nodes: RFNode[]; edges: RFEdge[] } {
  return {
    nodes: graph.nodes.map((node) => toRFNode(node, registeredTypes)),
    edges: graph.edges.map(toRFEdge),
  }
}

export function fromRFGraph(nodes: RFNode[], edges: RFEdge[], viewport: Viewport): Graph {
  return {
    schema_version: SchemaVersion,
    nodes: nodes.map(fromRFNode),
    edges: edges.map(fromRFEdge).filter((edge): edge is GraphEdge => edge != null),
    viewport: { x: viewport.x, y: viewport.y, zoom: viewport.zoom },
  }
}
