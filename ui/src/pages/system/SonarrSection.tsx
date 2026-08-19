import { useState } from 'react'

import {
  queryKeys,
  useDeleteSonarrInstance,
  useUpsertSonarrInstance,
} from '../../api/queries'
import { storageModes, type SonarrInstance } from '../../api/types'
import { Button, Card, EmptyState, Row } from '../../components/Card'
import {
  EditableNumber,
  EditableSelect,
  EditableText,
} from '../../components/Editable'

/*
 * Sonarr instances. Like scan directories these are keyed by a slug the upsert
 * matches on, so the slug is fixed once created.
 *
 * Root directory mappings translate a path as Sonarr reports it into one on
 * this machine — the pair only means something together, so they are edited as
 * rows rather than as two independent lists.
 */
export function SonarrSection({
  instances,
}: {
  instances: SonarrInstance[]
}) {
  const upsert = useUpsertSonarrInstance()
  const remove = useDeleteSonarrInstance()
  const [adding, setAdding] = useState(false)

  return (
    <Card
      title="Sonarr interfaces"
      description="Sonarr instances Metarr caches series data from."
      actions={
        <Button variant="primary" onClick={() => setAdding(true)}>
          Add instance
        </Button>
      }
    >
      {instances.length === 0 && !adding ? (
        <EmptyState>No Sonarr instances configured</EmptyState>
      ) : null}

      <div className="flex flex-col gap-4">
        {instances.map((instance) => (
          <InstanceCard
            key={instance.instance_slug}
            instance={instance}
            onSave={(next) => upsert.mutateAsync(next)}
            onRemove={() => {
              if (
                window.confirm(
                  `Remove the Sonarr instance "${instance.instance_name || instance.instance_slug}"?`,
                )
              ) {
                void remove.mutateAsync(instance.instance_slug)
              }
            }}
          />
        ))}
      </div>

      {adding ? (
        <NewInstance
          existingSlugs={instances.map((entry) => entry.instance_slug)}
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

function InstanceCard({
  instance,
  onSave,
  onRemove,
}: {
  instance: SonarrInstance
  onSave: (next: SonarrInstance) => Promise<unknown>
  onRemove: () => void
}) {
  const key = queryKeys.sonarr

  return (
    <div className="rounded border border-edge px-4 py-2">
      <div className="flex items-center justify-between gap-3 border-b border-edge/60 pb-2">
        <span className="font-mono text-sm text-ink-strong">
          {instance.instance_slug}
        </span>
        <Button variant="danger" onClick={onRemove}>
          Remove
        </Button>
      </div>

      <Row label="Name">
        <EditableText
          label="Instance name"
          queryKey={key}
          value={instance.instance_name}
          placeholder="Unnamed instance"
          onSave={(instance_name) => onSave({ ...instance, instance_name })}
        />
      </Row>

      <Row label="URL">
        <EditableText
          label="Sonarr URL"
          queryKey={key}
          value={instance.sonarr_url}
          monospace
          placeholder="http://localhost:8989"
          validate={(next) =>
            next.startsWith('http://') || next.startsWith('https://')
              ? null
              : 'Must start with http:// or https://'
          }
          onSave={(sonarr_url) => onSave({ ...instance, sonarr_url })}
        />
      </Row>

      <Row label="API key">
        <EditableText
          label="Sonarr API key"
          queryKey={key}
          value={instance.sonarr_api_key}
          monospace
          secret
          placeholder="No key set"
          onSave={(sonarr_api_key) => onSave({ ...instance, sonarr_api_key })}
        />
      </Row>

      <Row
        label="Storage mode"
        hint="cache expires after a TTL; versioned keeps revisions"
      >
        <EditableSelect
          label="Storage mode"
          queryKey={key}
          value={instance.storage?.mode ?? 'cache'}
          options={storageModes}
          onSave={(mode) =>
            onSave({ ...instance, storage: { ...instance.storage, mode } })
          }
        />
      </Row>

      {/* Only the field belonging to the active mode is meaningful, so only
          that one is offered — showing both invites setting a value that is
          silently ignored. */}
      {instance.storage?.mode === 'versioned' ? (
        <Row label="Max revisions">
          <EditableNumber
            label="Max count"
            queryKey={key}
            value={instance.storage?.max_count ?? 0}
            min={1}
            onSave={(max_count) =>
              onSave({
                ...instance,
                storage: { ...instance.storage, max_count },
              })
            }
          />
        </Row>
      ) : (
        // The server stores this string without parsing it today, so the editor
        // does not enforce a format — rejecting a value the API accepts would
        // make an existing entry uneditable.
        <Row label="TTL" hint="How long cached data lives, e.g. 24h or 90m">
          <EditableText
            label="TTL"
            queryKey={key}
            value={instance.storage?.ttl ?? ''}
            monospace
            placeholder="24h"
            onSave={(ttl) =>
              onSave({ ...instance, storage: { ...instance.storage, ttl } })
            }
          />
        </Row>
      )}

      <Row
        label="Root directory map"
        hint="Sonarr's path on the left, this machine's on the right"
      >
        <RootDirMap instance={instance} onSave={onSave} />
      </Row>
    </div>
  )
}

function RootDirMap({
  instance,
  onSave,
}: {
  instance: SonarrInstance
  onSave: (next: SonarrInstance) => Promise<unknown>
}) {
  const mappings = instance.root_dir_map ?? []
  const [adding, setAdding] = useState(false)
  const [sonarrPath, setSonarrPath] = useState('')
  const [localPath, setLocalPath] = useState('')

  function write(root_dir_map: SonarrInstance['root_dir_map']) {
    return onSave({ ...instance, root_dir_map })
  }

  return (
    <div className="flex flex-col gap-2">
      {mappings.length === 0 && !adding ? (
        <span className="text-sm text-ink-muted italic">No mappings</span>
      ) : null}

      {mappings.map((mapping, index) => (
        <div key={index} className="flex flex-wrap items-center gap-2">
          <div className="min-w-0 flex-1">
            <EditableText
              label="Sonarr path"
              queryKey={queryKeys.sonarr}
              value={mapping.sonarr_path}
              monospace
              onSave={(sonarr_path) => {
                const next = [...mappings]
                next[index] = { ...mapping, sonarr_path }
                return write(next)
              }}
            />
          </div>
          <span aria-hidden="true" className="text-ink-muted">
            →
          </span>
          <div className="min-w-0 flex-1">
            <EditableText
              label="Local path"
              queryKey={queryKeys.sonarr}
              value={mapping.local_path}
              monospace
              onSave={(local_path) => {
                const next = [...mappings]
                next[index] = { ...mapping, local_path }
                return write(next)
              }}
            />
          </div>
          <Button
            variant="danger"
            onClick={() => void write(mappings.filter((_, i) => i !== index))}
          >
            ×
          </Button>
        </div>
      ))}

      {adding ? (
        <div className="flex flex-wrap items-center gap-2">
          <input
            autoFocus
            value={sonarrPath}
            placeholder="/tv"
            onChange={(event) => setSonarrPath(event.target.value)}
            className="min-w-0 flex-1 rounded border border-edge-strong/40 bg-canvas px-2 py-1 font-mono text-sm text-ink-strong focus:border-blue"
          />
          <span aria-hidden="true" className="text-ink-muted">
            →
          </span>
          <input
            value={localPath}
            placeholder="/media/tv"
            onChange={(event) => setLocalPath(event.target.value)}
            className="min-w-0 flex-1 rounded border border-edge-strong/40 bg-canvas px-2 py-1 font-mono text-sm text-ink-strong focus:border-blue"
          />
          <Button
            variant="primary"
            disabled={!sonarrPath.trim() || !localPath.trim()}
            onClick={() => {
              void write([
                ...mappings,
                {
                  sonarr_path: sonarrPath.trim(),
                  local_path: localPath.trim(),
                },
              ])
              setSonarrPath('')
              setLocalPath('')
              setAdding(false)
            }}
          >
            Add
          </Button>
          <Button variant="ghost" onClick={() => setAdding(false)}>
            Cancel
          </Button>
        </div>
      ) : (
        <div>
          <Button onClick={() => setAdding(true)}>Add mapping</Button>
        </div>
      )}
    </div>
  )
}

function NewInstance({
  existingSlugs,
  onCreate,
  onCancel,
}: {
  existingSlugs: string[]
  onCreate: (entry: SonarrInstance) => Promise<void>
  onCancel: () => void
}) {
  const [slug, setSlug] = useState('')
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [error, setError] = useState<string | null>(null)

  async function submit() {
    if (!slug.trim()) {
      setError('A slug is required — it is how the API addresses this instance')
      return
    }
    if (existingSlugs.includes(slug.trim())) {
      setError('That slug is already in use; it would replace the existing instance')
      return
    }
    setError(null)
    await onCreate({
      instance_slug: slug.trim(),
      instance_name: name.trim() || slug.trim(),
      sonarr_url: url.trim(),
      sonarr_api_key: apiKey.trim(),
      root_dir_map: [],
      storage: { mode: 'cache', ttl: '24h' },
    })
  }

  return (
    <div className="mt-3 flex flex-col gap-2 rounded border border-dashed border-blue/60 px-4 py-3">
      <input
        autoFocus
        value={slug}
        placeholder="Slug, e.g. sonarr-main"
        onChange={(event) => setSlug(event.target.value)}
        className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 font-mono text-sm text-ink-strong focus:border-blue"
      />
      <input
        value={name}
        placeholder="Display name"
        onChange={(event) => setName(event.target.value)}
        className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm text-ink-strong focus:border-blue"
      />
      <input
        value={url}
        placeholder="http://localhost:8989"
        onChange={(event) => setUrl(event.target.value)}
        className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 font-mono text-sm text-ink-strong focus:border-blue"
      />
      <input
        value={apiKey}
        placeholder="Sonarr API key"
        onChange={(event) => setApiKey(event.target.value)}
        className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 font-mono text-sm text-ink-strong focus:border-blue"
      />

      {error ? <span className="text-xs text-red">{error}</span> : null}

      <div className="flex gap-2">
        <Button variant="primary" onClick={() => void submit()}>
          Add
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  )
}
