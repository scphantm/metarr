import { useState } from 'react'

import { queryKeys, useUpdateConfig } from '../../api/queries'
import { apiKeyGroups, type APIKeyGroup, type Config } from '../../api/types'
import { Button, Card, EmptyState } from '../../components/Card'
import { EditableText } from '../../components/Editable'

/*
 * API keys have no endpoint of their own — they live inside the config
 * document, so every edit here read-modify-writes the whole thing through
 * PUT /api/config.
 *
 * The keys come back from the server in cleartext, so they are masked until
 * asked for. Editing one in place is genuinely useful: this is where a key is
 * pasted in after being generated elsewhere.
 */
export function ApiKeysSection({ config }: { config: Config }) {
  const updateConfig = useUpdateConfig()
  const [addingTo, setAddingTo] = useState<APIKeyGroup | null>(null)
  const [draftName, setDraftName] = useState('')

  function writeGroup(
    group: APIKeyGroup,
    entries: Config['api_keys'][APIKeyGroup],
  ) {
    return updateConfig.mutateAsync({
      ...config,
      api_keys: { ...config.api_keys, [group]: entries },
    })
  }

  return (
    <Card
      title="API keys"
      description="Static keys, grouped by the access each grants. A key is shown only when you ask for it."
    >
      <div className="flex flex-col gap-6">
        {apiKeyGroups.map(({ key: group, label, hint }) => {
          const entries = config.api_keys[group] ?? []

          return (
            <div key={group}>
              <div className="mb-2 flex items-baseline justify-between gap-3">
                <div>
                  <h3 className="text-sm text-ink-strong">{label}</h3>
                  <p className="text-xs text-ink-muted">{hint}</p>
                </div>
                <Button
                  onClick={() => {
                    setAddingTo(group)
                    setDraftName('')
                  }}
                >
                  Add key
                </Button>
              </div>

              {entries.length === 0 && addingTo !== group ? (
                <EmptyState>No {label.toLowerCase()} keys</EmptyState>
              ) : (
                <div className="flex flex-col gap-1">
                  {entries.map((entry, index) => (
                    <div
                      key={`${group}-${index}`}
                      className="flex flex-wrap items-center gap-2 rounded border border-edge px-2 py-1.5"
                    >
                      <div className="w-40 shrink-0">
                        <EditableText
                          label="Key name"
                          queryKey={queryKeys.config}
                          value={entry.name}
                          placeholder="Unnamed"
                          onSave={(name) => {
                            const next = [...entries]
                            next[index] = { ...entry, name }
                            return writeGroup(group, next)
                          }}
                        />
                      </div>
                      <div className="min-w-0 flex-1">
                        <EditableText
                          label="API key"
                          queryKey={queryKeys.config}
                          value={entry.api_key}
                          placeholder="No key set"
                          monospace
                          secret
                          onSave={(api_key) => {
                            const next = [...entries]
                            next[index] = { ...entry, api_key }
                            return writeGroup(group, next)
                          }}
                        />
                      </div>
                      <Button
                        variant="danger"
                        title={`Remove ${entry.name || 'this key'}`}
                        onClick={() =>
                          void writeGroup(
                            group,
                            entries.filter((_, i) => i !== index),
                          )
                        }
                      >
                        Remove
                      </Button>
                    </div>
                  ))}
                </div>
              )}

              {addingTo === group ? (
                <div className="mt-2 flex items-center gap-2 rounded border border-dashed border-blue/60 px-2 py-1.5">
                  <input
                    autoFocus
                    value={draftName}
                    placeholder="Name for the new key"
                    onChange={(event) => setDraftName(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === 'Escape') setAddingTo(null)
                      if (event.key === 'Enter' && draftName.trim()) {
                        void writeGroup(group, [
                          ...entries,
                          { name: draftName.trim(), api_key: '' },
                        ])
                        setAddingTo(null)
                      }
                    }}
                    className="flex-1 rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm text-ink-strong focus:border-blue"
                  />
                  <Button
                    variant="primary"
                    disabled={!draftName.trim()}
                    onClick={() => {
                      void writeGroup(group, [
                        ...entries,
                        { name: draftName.trim(), api_key: '' },
                      ])
                      setAddingTo(null)
                    }}
                  >
                    Add
                  </Button>
                  <Button variant="ghost" onClick={() => setAddingTo(null)}>
                    Cancel
                  </Button>
                </div>
              ) : null}
            </div>
          )
        })}
      </div>
    </Card>
  )
}
