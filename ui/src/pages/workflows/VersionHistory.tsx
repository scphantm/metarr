import { Menu } from 'antd'
import { timestampDate } from '@bufbuild/protobuf/wkt'

import type { Workflow } from '../../gen/metarr/v1/workflows_pb'
import './VersionHistory.css'

export function VersionHistory({
  versions,
  viewingVersion,
  onSelect,
}: {
  versions: Workflow[]
  viewingVersion: number | null
  onSelect: (version: number) => void
}) {
  if (versions.length === 0) return null

  return (
    <div className="version-history">
      <div className="version-history-heading">History</div>
      <Menu
        mode="vertical"
        selectable
        selectedKeys={viewingVersion !== null ? [String(viewingVersion)] : []}
        onClick={({ key }) => onSelect(Number(key))}
        className="version-history-menu"
        items={versions.map((version) => ({
          key: String(version.version),
          label: (
            <>
              v{version.version}
              <span className="version-history-timestamp">
                {version.createdAt
                  ? timestampDate(version.createdAt).toLocaleString()
                  : ''}
              </span>
            </>
          ),
        }))}
      />
    </div>
  )
}
