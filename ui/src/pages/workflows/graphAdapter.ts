import { create } from "@bufbuild/protobuf";
import type { Edge as RFEdge, Node as RFNode, Viewport } from "@xyflow/react";

import {
  WorkflowEdgeKind,
  WorkflowGraphEdgeSchema,
  WorkflowGraphNodeSchema,
  WorkflowGraphSchema,
  type WorkflowGraph,
  type WorkflowGraphEdge,
  type WorkflowGraphNode,
} from "../../gen/metarr/v1/workflow_graph_pb";
import { SchemaVersion, type CatalogNodeData } from "./editorNodeData";
import {
  controlHandleId,
  dataHandleId,
  parseHandleId,
} from "./connectionRules";

/*
 * The canonical-graph <-> React Flow boundary. Nothing else in the editor
 * should reach into a WorkflowGraphNode/WorkflowGraphEdge's shape directly,
 * or build/read a React Flow node's `type`/`data`/handle ids by hand — this
 * is the one place that translation happens.
 *
 * The graph is a generated message now (docs/adr/0005). Its node carries two
 * structured passthrough fields, `settings` and `extra`, so a node whose
 * type this build does not recognise and settings it does not recognise
 * survive a save. The editor's per-instance colour overrides have no field
 * on the message, so they ride in `extra` — read back out here and written
 * back in on save, which keeps the rest of the editor working in plain data.
 */

export const UNKNOWN_NODE_TYPE = "unknownNode";

function extraString(
  extra: WorkflowGraphNode["extra"],
  key: string,
): string | undefined {
  const value = extra?.[key];
  return typeof value === "string" ? value : undefined;
}

// registeredTypes is the set of catalog types nodes/registry.ts has a real
// component for. A saved node whose type isn't in it — catalog drift,
// design.md §9 — renders through UnknownNode instead: the node and its
// settings/label are never dropped, just made visible as unrenderable.
export function toRFNode(
  node: WorkflowGraphNode,
  registeredTypes: ReadonlySet<string>,
): RFNode {
  const data: CatalogNodeData = {
    settings: node.settings ?? {},
    promoted: node.promoted ?? [],
    label: node.label || undefined,
    catalogId: node.catalogId || undefined,
    shapeColor: extraString(node.extra, "shapeColor"),
    borderColor: extraString(node.extra, "borderColor"),
  };
  const position = { x: node.position?.x ?? 0, y: node.position?.y ?? 0 };

  if (!registeredTypes.has(node.type)) {
    return {
      id: node.id,
      type: UNKNOWN_NODE_TYPE,
      position,
      data: { ...data, catalogType: node.type },
    };
  }

  return { id: node.id, type: node.type, position, data };
}

export function fromRFNode(node: RFNode): WorkflowGraphNode {
  const data = node.data as Partial<CatalogNodeData> & { catalogType?: string };
  const type = data.catalogType ?? node.type ?? "";

  // Anything the editor keeps but the message has no field for goes into
  // extra verbatim, so a round trip through storage does not lose it.
  const extra: Record<string, unknown> = {};
  if (data.shapeColor) extra.shapeColor = data.shapeColor;
  if (data.borderColor) extra.borderColor = data.borderColor;

  const graphNode = create(WorkflowGraphNodeSchema, {
    id: node.id,
    type,
    catalogId: data.catalogId ?? "",
    position: { x: node.position.x, y: node.position.y },
    promoted: data.promoted && data.promoted.length > 0 ? data.promoted : [],
    label: data.label ?? "",
  });
  if (data.settings && Object.keys(data.settings).length > 0) {
    graphNode.settings = data.settings as WorkflowGraphNode["settings"];
  }
  if (Object.keys(extra).length > 0) {
    graphNode.extra = extra as WorkflowGraphNode["extra"];
  }
  return graphNode;
}

export function toRFEdge(edge: WorkflowGraphEdge): RFEdge {
  const isControl = edge.kind === WorkflowEdgeKind.CONTROL;
  const fromPort = edge.from?.port ?? "";
  const toPort = edge.to?.port ?? "";
  const data =
    edge.transform || edge.settings
      ? { transform: edge.transform || undefined, settings: edge.settings }
      : undefined;
  return {
    id: edge.id,
    type: isControl ? "controlEdge" : "dataEdge",
    source: edge.from?.node ?? "",
    sourceHandle: isControl
      ? controlHandleId(fromPort)
      : dataHandleId(fromPort),
    target: edge.to?.node ?? "",
    targetHandle: isControl ? controlHandleId(toPort) : dataHandleId(toPort),
    data,
  };
}

export function fromRFEdge(edge: RFEdge): WorkflowGraphEdge | null {
  const from = parseHandleId(edge.sourceHandle);
  const to = parseHandleId(edge.targetHandle);
  if (!from || !to || from.kind !== to.kind) return null;

  const graphEdge = create(WorkflowGraphEdgeSchema, {
    id: edge.id,
    kind:
      from.kind === "control"
        ? WorkflowEdgeKind.CONTROL
        : WorkflowEdgeKind.DATA,
    from: { node: edge.source, port: from.name },
    to: { node: edge.target, port: to.name },
  });
  const data = edge.data as
    { transform?: string; settings?: Record<string, unknown> } | undefined;
  if (data?.transform) graphEdge.transform = data.transform;
  if (data?.settings && Object.keys(data.settings).length > 0) {
    graphEdge.settings = data.settings as WorkflowGraphEdge["settings"];
  }
  return graphEdge;
}

export function toRFGraph(
  graph: Pick<WorkflowGraph, "nodes" | "edges">,
  registeredTypes: ReadonlySet<string>,
): { nodes: RFNode[]; edges: RFEdge[] } {
  return {
    nodes: graph.nodes.map((node) => toRFNode(node, registeredTypes)),
    edges: graph.edges.map(toRFEdge),
  };
}

export function fromRFGraph(
  nodes: RFNode[],
  edges: RFEdge[],
  viewport: Viewport,
): WorkflowGraph {
  return create(WorkflowGraphSchema, {
    schemaVersion: SchemaVersion,
    nodes: nodes.map(fromRFNode),
    edges: edges
      .map(fromRFEdge)
      .filter((edge): edge is WorkflowGraphEdge => edge != null),
    viewport: { x: viewport.x, y: viewport.y, zoom: viewport.zoom },
  });
}
