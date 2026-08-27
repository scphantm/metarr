import { useState } from 'react'
import { Input, Select, Space, Typography } from 'antd'

import {
  queryKeys,
  useDeleteScanDirectory,
  useUpsertScanDirectory,
} from '../../api/queries'
import { directoryTypes, type ScanDirectory } from '../../api/types'
import { Button, Card, EmptyState, Row } from '../../components/Card'
import {
  EditableSelect,
  EditableText,
} from '../../components/Editable'
import './ScanDirectoriesSection.css'

/*
 * Scan directories are keyed by scanner_slug, which the upsert endpoint matches
 * on. That makes the slug the one field that cannot be edited in place: writing
 * a new slug would create a second entry rather than rename this one, silently
 * leaving the original behind.
 */
export function ScanDirectoriesSection({
  directories,
}: {
  directories: ScanDirectory[]
}) {
  const upsert = useUpsertScanDirectory()
  const remove = useDeleteScanDirectory()
  const [adding, setAdding] = useState(false)

  return (
    <Card
      title="Scan directories"
      description="Library roots the scanner walks, each tagged with the kind of media it holds."
      actions={
        <Button variant="primary" onClick={() => setAdding(true)}>
          Add directory
        </Button>
      }
    >
      {directories.length === 0 && !adding ? (
        <EmptyState>
          No directories configured. The scanner has nothing to walk.
        </EmptyState>
      ) : null}

      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        {directories.map((directory) => (
          <div key={directory.scanner_slug} className="scan-directory-card">
            <div className="scan-directory-card-header">
              <Typography.Text className="scan-directory-slug">
                {directory.scanner_slug}
              </Typography.Text>
              <Button
                variant="danger"
                onClick={() => {
                  if (
                    window.confirm(
                      `Remove the scan directory "${directory.scanner_slug}"? Records already scanned from it are not deleted.`,
                    )
                  ) {
                    void remove.mutateAsync(directory.scanner_slug)
                  }
                }}
              >
                Remove
              </Button>
            </div>

            <Row label="Path">
              <EditableText
                label="Directory path"
                queryKey={queryKeys.scanDirectories}
                value={directory.directory}
                monospace
                placeholder="/media/movies"
                validate={(next) =>
                  next.startsWith('/') ? null : 'Must be an absolute path'
                }
                onSave={(value) =>
                  upsert.mutateAsync({ ...directory, directory: value })
                }
              />
            </Row>

            <Row label="Media type">
              <EditableSelect
                label="Scan type"
                queryKey={queryKeys.scanDirectories}
                value={directory.scan_type}
                options={directoryTypes}
                onSave={(value) =>
                  upsert.mutateAsync({ ...directory, scan_type: value })
                }
              />
            </Row>
          </div>
        ))}
      </Space>

      {adding ? (
        <NewScanDirectory
          existingSlugs={directories.map((entry) => entry.scanner_slug)}
          onCancel={() => setAdding(false)}
          onCreate={async (entry) => {
            await upsert.mutateAsync(entry)
            setAdding(false)
          }}
        />
      ) : null}
    </Card>
  )
}

function NewScanDirectory({
  existingSlugs,
  onCreate,
  onCancel,
}: {
  existingSlugs: string[]
  onCreate: (entry: ScanDirectory) => Promise<void>
  onCancel: () => void
}) {
  const [slug, setSlug] = useState('')
  const [directory, setDirectory] = useState('')
  const [scanType, setScanType] = useState<string>(directoryTypes[0])
  const [error, setError] = useState<string | null>(null)

  async function submit() {
    if (!slug.trim()) {
      setError('A slug is required — it is how the API addresses this entry')
      return
    }
    if (existingSlugs.includes(slug.trim())) {
      setError('That slug is already in use; it would replace the existing entry')
      return
    }
    if (!directory.startsWith('/')) {
      setError('The path must be absolute')
      return
    }
    setError(null)
    await onCreate({
      scanner_slug: slug.trim(),
      directory: directory.trim(),
      scan_type: scanType,
    })
  }

  return (
    <div className="new-scan-directory">
      <Input
        autoFocus
        value={slug}
        placeholder="Slug, e.g. movies-4k"
        className="editable-field-mono"
        onChange={(event) => setSlug(event.target.value)}
      />
      <Input
        value={directory}
        placeholder="/media/movies"
        className="editable-field-mono"
        onChange={(event) => setDirectory(event.target.value)}
      />
      <Select
        value={scanType}
        style={{ width: 192 }}
        onChange={setScanType}
        options={directoryTypes.map((type) => ({ value: type, label: type }))}
      />

      {error ? (
        <Typography.Text type="danger" style={{ fontSize: 12 }}>
          {error}
        </Typography.Text>
      ) : null}

      <Space>
        <Button variant="primary" onClick={() => void submit()}>
          Add
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </Space>
    </div>
  )
}
