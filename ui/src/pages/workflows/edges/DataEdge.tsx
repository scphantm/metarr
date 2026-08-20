import { useState } from 'react'
import { BaseEdge, EdgeLabelRenderer, getBezierPath, useReactFlow, type Edge, type EdgeProps } from '@xyflow/react'

import { typeStrokeColor } from '../../../lib/typeColors'
import { canConnect, parseHandleId } from '../connectionRules'
import { useCatalogEntry, useTransforms } from '../useCatalogEntry'
import { TransformPicker } from './TransformPicker'

/*
 * The one shared data-edge component: thin, static, colored by the source
 * socket's type (lib/typeColors.ts), dashed when a transform is applied.
 * The transform — if any — shows as a clickable chip (design.md §4.4's
 * "small chip on the edge reading e.g. parentDir"); clicking it reopens
 * TransformPicker to change or clear the conversion.
 */

export type DataEdgeData = { transform?: string }
export type DataEdgeType = Edge<DataEdgeData, 'dataEdge'>

export function DataEdge({
  id,
  source,
  target,
  sourceHandleId,
  targetHandleId,
  sourceX,
  sourceY,
  sourcePosition,
  targetX,
  targetY,
  targetPosition,
  data,
  markerEnd,
  selected,
}: EdgeProps<DataEdgeType>) {
  const { getNode, updateEdgeData } = useReactFlow()
  const [pickerOpen, setPickerOpen] = useState(false)
  const transforms = useTransforms()

  const sourceNode = getNode(source)
  const targetNode = getNode(target)
  const sourceNodeType = useCatalogEntry(sourceNode?.type ?? '')
  const targetNodeType = useCatalogEntry(targetNode?.type ?? '')

  const sourceSocketName = parseHandleId(sourceHandleId)?.name
  const targetSocketName = parseHandleId(targetHandleId)?.name
  const sourceSocket = sourceNodeType?.dataOut?.find((socket) => socket.name === sourceSocketName)
  const targetSocket = targetNodeType?.dataIn?.find((socket) => socket.name === targetSocketName)

  const color = typeStrokeColor(sourceSocket?.type ?? 'any')
  const [path, labelX, labelY] = getBezierPath({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition })

  return (
    <>
      <BaseEdge
        id={id}
        path={path}
        markerEnd={markerEnd}
        style={{
          stroke: color,
          strokeWidth: 1.5,
          strokeDasharray: data?.transform ? '4 3' : undefined,
          opacity: selected ? 1 : 0.85,
        }}
      />

      {data?.transform ? (
        <EdgeLabelRenderer>
          <div
            style={{ position: 'absolute', transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
            className="nodrag nopan pointer-events-auto"
          >
            <button
              type="button"
              onClick={() => setPickerOpen(true)}
              className="rounded border border-edge-strong/50 bg-surface px-1.5 py-0.5 font-mono text-[10px] text-ink-strong shadow-sm hover:border-blue"
            >
              {data.transform}
            </button>
          </div>
        </EdgeLabelRenderer>
      ) : null}

      {pickerOpen && sourceSocket && targetSocket
        ? (() => {
            const connection = canConnect(sourceSocket.type, targetSocket.type, transforms)
            return (
              <TransformPicker
                fromType={sourceSocket.type}
                toType={targetSocket.type}
                candidates={connection.candidates}
                current={data?.transform}
                onPick={(name) => {
                  void updateEdgeData(id, { transform: name })
                  setPickerOpen(false)
                }}
                onClose={() => setPickerOpen(false)}
              />
            )
          })()
        : null}
    </>
  )
}
