import { useState } from 'react'
import { Badge, Button, Space, Typography } from 'antd'
import { DownOutlined, RightOutlined } from '@ant-design/icons'

import type { Diagnostic } from './catalogTypes'
import './DiagnosticsPanel.css'

/*
 * Renders the debounced POST /api/workflows/validate result as a collapsible
 * list, docked inside the canvas via React Flow's own <Panel>. Clicking a
 * row (or a node in its witness path) pans/selects that node — see
 * WorkflowCanvas's onSelectDiagnosticNode. Hovering a row blinks the edge(s)
 * it names (diagnostic.edge_ids) via onHoverDiagnostic — see
 * WorkflowCanvas's hoveredDiagnosticEdgeIds and edges/ControlEdge.tsx /
 * edges/DataEdge.tsx's diagnosticHighlight.
 */
export function DiagnosticsPanel({
  diagnostics,
  nodeLabel,
  onSelectNode,
  onHoverDiagnostic,
}: {
  diagnostics: Diagnostic[]
  nodeLabel: (nodeId: string) => string
  onSelectNode: (nodeId: string) => void
  onHoverDiagnostic: (edgeIds: string[]) => void
}) {
  const [open, setOpen] = useState(false)

  const errorCount = diagnostics.filter((d) => d.severity === 'error').length
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
                  onMouseEnter={() => onHoverDiagnostic(diagnostic.edge_ids ?? [])}
                  onMouseLeave={() => onHoverDiagnostic([])}
                >
                  <div className="diagnostics-panel-item-row">
                    <span
                      className="diagnostics-panel-item-dot"
                      style={{
                        backgroundColor:
                          diagnostic.severity === 'error' ? 'var(--color-red)' : 'var(--color-yellow)',
                      }}
                    />
                    <div className="diagnostics-panel-item-main">
                      <p className="diagnostics-panel-item-message">{diagnostic.message}</p>
                      {diagnostic.node_ids && diagnostic.node_ids.length > 0 ? (
                        <div className="diagnostics-panel-node-chips">
                          {diagnostic.node_ids.map((nodeId) => (
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
                      {diagnostic.witness_path && diagnostic.witness_path.length > 0 ? (
                        <div className="diagnostics-panel-witness-path">
                          {diagnostic.witness_path.map((nodeId, pathIndex) => (
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
