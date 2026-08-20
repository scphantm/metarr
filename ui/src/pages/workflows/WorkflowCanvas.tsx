import { useCallback, useEffect, useMemo, useState, type DragEvent } from 'react'
import {
  addEdge,
  Background,
  Controls,
  Panel,
  ReactFlow,
  useEdgesState,
  useNodesState,
  useReactFlow,
  type Connection,
  type Edge,
  type Node,
  type ReactFlowInstance,
  type Viewport,
} from '@xyflow/react'

import { useWorkflowCatalog } from '../../api/queries'
import { autoApplyTransform, evaluateConnection } from './connectionRules'
import { DiagnosticsPanel } from './DiagnosticsPanel'
import { useDnD } from './DnDContext'
import { ControlEdge } from './edges/ControlEdge'
import { DataEdge } from './edges/DataEdge'
import { TransformPicker } from './edges/TransformPicker'
import { fromRFGraph, toRFNode } from './graphAdapter'
import { nodeTypes as catalogNodeTypes, registeredTypes, unknownNodeType } from './nodes/registry'
import type { Accent } from './nodes/shared/nodeVisual'
import { useWorkflowValidation } from './useWorkflowValidation'
import type { NodeType, Transform } from './catalogTypes'

const nodeTypes = { ...catalogNodeTypes, ...unknownNodeType }
const edgeTypes = { controlEdge: ControlEdge, dataEdge: DataEdge }

// The same 8 accents nodeVisual.ts resolves node colors from — see that
// file's ACCENT-keyed maps. Order here is just the cycle order, not
// meaningful otherwise.
const ACCENTS: Accent[] = ['red', 'orange', 'yellow', 'green', 'cyan', 'blue', 'violet', 'magenta']

type PendingPicker = {
  connection: Connection
  fromType: string
  toType: string
  candidates: Transform[]
}

export function WorkflowCanvas({
  initialNodes,
  initialEdges,
  initialViewport,
  readOnly,
  onInit,
}: {
  initialNodes: Node[]
  initialEdges: Edge[]
  // Undefined for a brand-new workflow, which still wants fitView instead —
  // see the defaultViewport/fitView split below. Previously persisted
  // through every save but never applied on load; restoring it here fixes
  // that.
  initialViewport: Viewport | undefined
  readOnly: boolean
  onInit: (instance: ReactFlowInstance) => void
}) {
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)
  const { screenToFlowPosition, getNode, fitView } = useReactFlow()
  const { setDraggedTemplate } = useDnD()
  const { data: catalog } = useWorkflowCatalog()

  const [pendingPicker, setPendingPicker] = useState<PendingPicker | null>(null)
  const [connectionError, setConnectionError] = useState<string | null>(null)

  useEffect(() => {
    if (!connectionError) return
    const timer = window.setTimeout(() => setConnectionError(null), 4000)
    return () => window.clearTimeout(timer)
  }, [connectionError])

  // Exercises the quadrant notification layer (NodeShell.tsx) — every
  // node's four quadrants cycle through the 8 theme accents together, each
  // quadrant offset from the next so the whole node visibly rotates rather
  // than flashing one color at once. quadrantColors is deliberately absent
  // from GraphNode/graphAdapter.ts (see CatalogNodeData's own comment) — a
  // saved workflow never carries this. Unchecking clears every node back to
  // invisible rather than leaving whatever the last tick happened to set.
  const [testAnimate, setTestAnimate] = useState(false)
  useEffect(() => {
    if (!testAnimate) {
      setNodes((current) =>
        current.map((node) => ({ ...node, data: { ...node.data, quadrantColors: undefined } })),
      )
      return
    }
    let tick = 0
    const interval = window.setInterval(() => {
      tick += 1
      const quadrantColors = [0, 1, 2, 3].map((quadrant) => ACCENTS[(tick + quadrant * 2) % ACCENTS.length])
      setNodes((current) => current.map((node) => ({ ...node, data: { ...node.data, quadrantColors } })))
    }, 600)
    return () => window.clearInterval(interval)
  }, [testAnimate, setNodes])

  const catalogByType = useMemo(() => {
    const map = new Map<string, NodeType>()
    for (const entry of catalog?.node_types ?? []) map.set(entry.type, entry)
    return map
  }, [catalog])
  const transforms = useMemo(() => catalog?.transforms ?? [], [catalog])

  const endpointTypes = useCallback(
    (connection: Connection) => {
      const sourceNode = getNode(connection.source)
      const targetNode = getNode(connection.target)
      return {
        sourceType: sourceNode ? catalogByType.get(sourceNode.type ?? '') : undefined,
        targetType: targetNode ? catalogByType.get(targetNode.type ?? '') : undefined,
      }
    },
    [getNode, catalogByType],
  )

  const isValidConnection = useCallback(
    (candidate: Connection | Edge) => {
      const connection = candidate as Connection
      const { sourceType, targetType } = endpointTypes(connection)
      return evaluateConnection(connection, sourceType, targetType, edges, transforms).allowed
    },
    [edges, transforms, endpointTypes],
  )

  const onConnect = useCallback(
    (connection: Connection) => {
      const { sourceType, targetType } = endpointTypes(connection)
      const verdict = evaluateConnection(connection, sourceType, targetType, edges, transforms)

      if (!verdict.allowed) {
        setConnectionError(verdict.reason)
        return
      }
      if (verdict.kind === 'control') {
        setEdges((current) => addEdge({ ...connection, id: crypto.randomUUID(), type: 'controlEdge' }, current))
        return
      }
      if (verdict.connection.direct) {
        setEdges((current) => addEdge({ ...connection, id: crypto.randomUUID(), type: 'dataEdge' }, current))
        return
      }
      const auto = autoApplyTransform(verdict.connection)
      if (auto) {
        setEdges((current) =>
          addEdge(
            { ...connection, id: crypto.randomUUID(), type: 'dataEdge', data: { transform: auto.name } },
            current,
          ),
        )
        return
      }
      // design.md §4.4: several/ambiguous candidates get an inline picker
      // with nothing pre-selected; the edge does not exist until one is
      // chosen, and closing the picker without picking leaves it uncreated.
      setPendingPicker({
        connection,
        fromType: verdict.fromType,
        toType: verdict.toType,
        candidates: verdict.connection.candidates,
      })
    },
    [edges, transforms, endpointTypes, setEdges],
  )

  const onDragOver = useCallback((event: DragEvent) => {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
  }, [])

  const onDrop = useCallback(
    (event: DragEvent) => {
      event.preventDefault()
      const raw = event.dataTransfer.getData('application/json')
      if (!raw) return

      const dragged = JSON.parse(raw) as { type: string; typeVersion: string }
      const nodeType = catalogByType.get(dragged.type)
      if (!nodeType) return

      const position = screenToFlowPosition({ x: event.clientX, y: event.clientY })
      const defaultSettings: Record<string, unknown> = {}
      for (const setting of nodeType.settings ?? []) {
        if (setting.default !== undefined) defaultSettings[setting.name] = setting.default
      }

      const newNode = toRFNode(
        {
          id: crypto.randomUUID(),
          type: nodeType.type,
          typeVersion: nodeType.typeVersion,
          position,
          settings: defaultSettings,
        },
        registeredTypes,
      )
      setNodes((current) => current.concat(newNode))
      setDraggedTemplate(null)
    },
    [screenToFlowPosition, setNodes, setDraggedTemplate, catalogByType],
  )

  // Stamps readOnly onto each node's own data so a node can hide its Edit
  // button while a historic version is being viewed — the same gate
  // nodesDraggable/elementsSelectable already apply below, but those don't
  // reach into a custom node's own interactive controls.
  const displayNodes = useMemo(
    () => (readOnly ? nodes.map((node) => ({ ...node, data: { ...node.data, readOnly } })) : nodes),
    [nodes, readOnly],
  )

  const graph = useMemo(() => fromRFGraph(nodes, edges, { x: 0, y: 0, zoom: 1 }), [nodes, edges])
  const validation = useWorkflowValidation(graph, !readOnly)

  const nodeLabel = useCallback(
    (nodeId: string) => {
      const node = getNode(nodeId)
      const data = node?.data as { label?: string } | undefined
      return data?.label ?? (node ? catalogByType.get(node.type ?? '')?.name : undefined) ?? nodeId
    },
    [getNode, catalogByType],
  )

  const onSelectDiagnosticNode = useCallback(
    (nodeId: string) => {
      const node = getNode(nodeId)
      if (node) void fitView({ nodes: [node], duration: 300, maxZoom: 1.2 })
    },
    [getNode, fitView],
  )

  return (
    <div className="h-full w-full" onDrop={onDrop} onDragOver={onDragOver}>
      <ReactFlow
        nodes={displayNodes}
        edges={edges}
        onNodesChange={readOnly ? undefined : onNodesChange}
        onEdgesChange={readOnly ? undefined : onEdgesChange}
        onConnect={readOnly ? undefined : onConnect}
        isValidConnection={readOnly ? undefined : isValidConnection}
        onInit={onInit}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        nodesDraggable={!readOnly}
        nodesConnectable={!readOnly}
        edgesReconnectable={!readOnly}
        elementsSelectable={!readOnly}
        defaultViewport={initialViewport}
        fitView={!initialViewport}
      >
        <Background />
        <Controls />
        <Panel position="top-left">
          <label className="flex items-center gap-1.5 rounded border border-edge-strong/40 bg-surface px-2.5 py-1.5 text-xs text-ink-strong shadow-sm">
            <input
              type="checkbox"
              checked={testAnimate}
              onChange={(event) => setTestAnimate(event.target.checked)}
              className="h-3.5 w-3.5"
            />
            Test animate
          </label>
        </Panel>
        {connectionError ? (
          <Panel position="top-center">
            <div className="rounded border border-red/50 bg-surface px-3 py-1.5 text-xs text-red shadow-lg">
              {connectionError}
            </div>
          </Panel>
        ) : null}
        {!readOnly ? (
          <Panel position="bottom-right">
            <DiagnosticsPanel diagnostics={validation.diagnostics} nodeLabel={nodeLabel} onSelectNode={onSelectDiagnosticNode} />
          </Panel>
        ) : null}
      </ReactFlow>

      {pendingPicker ? (
        <TransformPicker
          fromType={pendingPicker.fromType}
          toType={pendingPicker.toType}
          candidates={pendingPicker.candidates}
          onPick={(name) => {
            setEdges((current) =>
              addEdge(
                { ...pendingPicker.connection, id: crypto.randomUUID(), type: 'dataEdge', data: { transform: name } },
                current,
              ),
            )
            setPendingPicker(null)
          }}
          onClose={() => setPendingPicker(null)}
        />
      ) : null}
    </div>
  )
}
