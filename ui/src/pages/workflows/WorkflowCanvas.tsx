import { useCallback, type DragEvent } from 'react'
import {
  addEdge,
  Background,
  Controls,
  ReactFlow,
  useEdgesState,
  useNodesState,
  useReactFlow,
  type Connection,
  type Edge,
  type Node,
  type ReactFlowInstance,
} from '@xyflow/react'

import { AnimatedSvgEdge } from '../../components/animated-svg-edge'
import type { NodeCatalogEntry } from './nodeCatalogTypes'
import { WorkflowNode, type WorkflowNodeData, type WorkflowNodeType } from './nodes/WorkflowNode'
import { useDnD } from './DnDContext'

const nodeTypes = { catalogNode: WorkflowNode }
const edgeTypes = { animatedSvgEdge: AnimatedSvgEdge }
const defaultEdgeOptions = {
  type: 'animatedSvgEdge',
  data: { duration: 2, shape: 'circle' as const, path: 'bezier' as const },
}

export function WorkflowCanvas({
  initialNodes,
  initialEdges,
  readOnly,
  onInit,
}: {
  initialNodes: Node[]
  initialEdges: Edge[]
  readOnly: boolean
  onInit: (instance: ReactFlowInstance) => void
}) {
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)
  const { screenToFlowPosition } = useReactFlow()
  const { setDraggedTemplate } = useDnD()

  const onConnect = useCallback(
    (connection: Connection) => setEdges((current) => addEdge(connection, current)),
    [setEdges],
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

      const template = JSON.parse(raw) as NodeCatalogEntry
      const position = screenToFlowPosition({ x: event.clientX, y: event.clientY })

      const data: WorkflowNodeData = {
        name: template.name,
        sourceRepo: template.sourceRepo,
        pluginName: template.pluginName,
        version: template.version,
        category: template.category,
        inputsDB: template.inputsDB,
      }
      // A fresh id per drop, not React Flow's demo-style incrementing
      // counter: a counter reset on every page load would collide with an
      // already-loaded workflow's existing node ids when editing.
      const newNode: WorkflowNodeType = {
        id: crypto.randomUUID(),
        type: 'catalogNode',
        position,
        data,
      }
      setNodes((current) => current.concat(newNode))
      setDraggedTemplate(null)
    },
    [screenToFlowPosition, setNodes, setDraggedTemplate],
  )

  return (
    <div className="h-full w-full" onDrop={onDrop} onDragOver={onDragOver}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={readOnly ? undefined : onNodesChange}
        onEdgesChange={readOnly ? undefined : onEdgesChange}
        onConnect={readOnly ? undefined : onConnect}
        onInit={onInit}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        defaultEdgeOptions={defaultEdgeOptions}
        nodesDraggable={!readOnly}
        nodesConnectable={!readOnly}
        edgesReconnectable={!readOnly}
        elementsSelectable={!readOnly}
        fitView
      >
        <Background />
        <Controls />
      </ReactFlow>
    </div>
  )
}
