import { Modal, Typography } from 'antd'

import type { WorkflowTransform as Transform } from '../../../gen/metarr/v1/workflow_catalog_pb'
import type { Type } from '../connectionRules'
import './TransformPicker.css'

/*
 * design.md §4.4, "several candidates": an inline picker with nothing
 * pre-selected. Shared by the new-connection flow (WorkflowCanvas.onConnect,
 * when canConnect() returns more than one candidate, or the sole candidate
 * is marked ambiguous) and by clicking an existing data edge's transform
 * chip to change it (DataEdge). A centered modal rather than a popover
 * pinned to the exact drop point — simpler to get right, and the choice
 * matters far more than its screen position.
 */
export function TransformPicker({
  fromType,
  toType,
  candidates,
  current,
  onPick,
  onClose,
}: {
  fromType: Type
  toType: Type
  candidates: Transform[]
  current?: string
  onPick: (name: string) => void
  onClose: () => void
}) {
  return (
    <Modal open title="Choose a conversion" onCancel={onClose} footer={null} width={384}>
      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        <span className="transform-picker-type">{fromType}</span> does not connect directly to{' '}
        <span className="transform-picker-type">{toType}</span>. Pick how to convert it:
      </Typography.Text>

      <div className="transform-picker-list">
        {candidates.map((transform) => (
          <button
            key={transform.name}
            type="button"
            onClick={() => onPick(transform.name)}
            className={`transform-picker-option ${transform.name === current ? 'is-current' : ''}`}
          >
            <div className="transform-picker-option-name">{transform.name}</div>
            {transform.summary ? (
              <div className="transform-picker-option-summary">{transform.summary}</div>
            ) : null}
          </button>
        ))}
      </div>
    </Modal>
  )
}
