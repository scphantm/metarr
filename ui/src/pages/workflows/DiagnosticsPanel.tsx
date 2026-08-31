import { useState } from 'react'
import { Badge, Button, Space, Typography } from 'antd'
import { DownOutlined, RightOutlined } from '@ant-design/icons'

import {
  type WorkflowDiagnostic,
  WorkflowDiagnosticSeverity,
} from '../../gen/metarr/v1/workflow_catalog_pb'
import './DiagnosticsPanel.css'

/*
 * Renders the debounced POST /api/workflows/validate result as a collapsible
 * list, docked inside the canvas via React Flow's own <Panel>. Clicking a
 * row (or a node in its witness path) pans/selects that node — see
 * WorkflowCanvas's onSelectDiagnosticNode. Hovering a row blinks the edge(s)
 * it names (diagnostic.edgeIds) via onHoverDiagnostic — see
 * WorkflowCanvas's hoveredDiagnosticEdgeIds and edges/ControlEdge.tsx /
 * edges/DataEdge.tsx's diagnosticHighlight.
 */
export function DiagnosticsPanel({
  diagnostics,
  nodeLabel,
  onSelectNode,
  onHoverDiagnostic,
}: {
  diagnostics: WorkflowDiagnostic[]
  nodeLabel: (nodeId: string) => string
  onSelectNode: (nodeId: string) => void
  onHoverDiagnostic: (edgeIds: string[]) => void
}) {
  const [open, setOpen] = useState(false)

  const errorCount = diagnostics.filter(
    (d) => d.severity === WorkflowDiagnosticSeverity.ERROR,
  ).length
  const warningCount = diagnostics.length - errorCount

  return (
    <div className="diagnostics-panel">
      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        className="diagnostics-panel-header"
      >
        <span>Diagnostics</span>
        <Space size={6} align="center">
          {errorCount > 0 ? <Badge count={errorCount} color="var(--color-red)" /> : null}
          {warningCount > 0 ? <Badge count={warningCount} color="var(--color-yellow)" /> : null}
          {diagnostics.length === 0 ? (
            <Typography.Text type="secondary" style={{ fontSize: 10 }}>
              clean
            </Typography.Text>
          ) : null}
          {open ? <DownOutlined style={{ fontSize: 10 }} /> : <RightOutlined style={{ fontSize: 10 }} />}
        </Space>
      </button>

      {open ? (
        <div className="diagnostics-panel-body">
          {diagnostics.length === 0 ? (
            <Typography.Text type="secondary" className="diagnostics-panel-empty">
              No issues.
            </Typography.Text>
          ) : (
            <ul className="diagnostics-panel-list">
              {diagnostics.map((diagnostic, index) => (
                <li
                  key={`${diagnostic.code}-${index}`}
                  className="diagnostics-panel-item"
                  onMouseEnter={() => onHoverDiagnostic(diagnostic.edgeIds)}
                  onMouseLeave={() => onHoverDiagnostic([])}
                >
                  <div className="diagnostics-panel-item-row">
                    <span
                      className="diagnostics-panel-item-dot"
                      style={{
                        backgroundColor:
                          diagnostic.severity === WorkflowDiagnosticSeverity.ERROR
                            ? 'var(--color-red)'
                            : 'var(--color-yellow)',
                      }}
                    />
                    <div className="diagnostics-panel-item-main">
                      <p className="diagnostics-panel-item-message">{diagnostic.message}</p>
                      {diagnostic.nodeIds.length > 0 ? (
                        <div className="diagnostics-panel-node-chips">
                          {diagnostic.nodeIds.map((nodeId) => (
                            <Button
                              key={nodeId}
                              size="small"
                              onClick={() => onSelectNode(nodeId)}
                              className="diagnostics-panel-node-chip"
                            >
                              {nodeLabel(nodeId)}
                            </Button>
                          ))}
                        </div>
                      ) : null}
                      {diagnostic.witnessPath.length > 0 ? (
                        <div className="diagnostics-panel-witness-path">
                          {diagnostic.witnessPath.map((nodeId, pathIndex) => (
                            <span key={`${nodeId}-${pathIndex}`} className="diagnostics-panel-witness-step">
                              {pathIndex > 0 ? <span>→</span> : null}
                              <Typography.Link onClick={() => onSelectNode(nodeId)} style={{ fontSize: 10 }}>
                                {nodeLabel(nodeId)}
                              </Typography.Link>
                            </span>
                          ))}
                        </div>
                      ) : null}
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : null}
    </div>
  )
}
