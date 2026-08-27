import { useState } from 'react'
import { Input, Space, Typography } from 'antd'

import { queryKeys, useUpdateConfig } from '../../api/queries'
import type { APIKeyEntry, APIKeysConfig, Config } from '../../gen/metarr/v1/config_pb'
import { Button, Card, EmptyState } from '../../components/Card'
import { EditableText } from '../../components/Editable'
import './ApiKeysSection.css'

// Not `keyof APIKeysConfig` — that also picks up $typeName/$unknown from the
// branded message type, neither of which is a real key group.
type APIKeyGroup = 'admin' | 'user' | 'webhook' | 'readOnly'

const apiKeyGroups: { key: APIKeyGroup; label: string; hint: string }[] = [
  { key: 'admin', label: 'Admin', hint: 'Full access to every endpoint' },
  { key: 'user', label: 'User', hint: 'Tasks and library reads' },
  { key: 'webhook', label: 'Webhook', hint: 'For inbound automation' },
  { key: 'readOnly', label: 'Read only', hint: 'Library reads only' },
]

/*
 * API keys have no endpoint of their own — they live inside the config
 * document, so every edit here read-modify-writes the whole thing through
 * ConfigService.Update.
 *
 * The keys come back from the server in cleartext, so they are masked until
 * asked for. Editing one in place is genuinely useful: this is where a key is
 * pasted in after being generated elsewhere.
 */
export function ApiKeysSection({ config }: { config: Config }) {
  const updateConfig = useUpdateConfig()
  const [addingTo, setAddingTo] = useState<APIKeyGroup | null>(null)
  const [draftName, setDraftName] = useState('')

  const apiKeys: APIKeysConfig = config.apiKeys ?? {
    $typeName: 'metarr.v1.APIKeysConfig',
    admin: [],
    user: [],
    webhook: [],
    readOnly: [],
  }

  // Never spread a branded protobuf message into a create() payload (breaks
  // MessageInitShape union inference) — build the next document field by
  // field instead, reusing every untouched top-level field as-is.
  function writeGroup(group: APIKeyGroup, entries: APIKeyEntry[]) {
    return updateConfig.mutateAsync({
      apiKeys: { ...apiKeys, [group]: entries },
      admin: config.admin,
      interfaces: config.interfaces,
      directoryScanner: config.directoryScanner,
      agents: config.agents,
      logging: config.logging,
    })
  }

  return (
    <Card
      title="API keys"
      description="Static keys, grouped by the access each grants. A key is shown only when you ask for it."
    >
      <Space direction="vertical" size={24} style={{ width: '100%' }}>
        {apiKeyGroups.map(({ key: group, label, hint }) => {
          const entries = apiKeys[group] ?? []

          return (
            <div key={group}>
              <div className="api-key-group-header">
                <div>
                  <Typography.Text className="api-key-group-label">{label}</Typography.Text>
                  <Typography.Text type="secondary" className="api-key-group-hint">
                    {hint}
                  </Typography.Text>
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
                <Space direction="vertical" size={4} style={{ width: '100%' }}>
                  {entries.map((entry, index) => (
                    <div key={`${group}-${index}`} className="api-key-row">
                      <div className="api-key-row-name">
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
                      <div className="api-key-row-value">
                        <EditableText
                          label="API key"
                          queryKey={queryKeys.config}
                          value={entry.apiKey}
                          placeholder="No key set"
                          monospace
                          secret
                          onSave={(apiKey) => {
                            const next = [...entries]
                            next[index] = { ...entry, apiKey }
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
                </Space>
              )}

              {addingTo === group ? (
                <div className="api-key-add-row">
                  <Input
                    autoFocus
                    value={draftName}
                    placeholder="Name for the new key"
                    onChange={(event) => setDraftName(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === 'Escape') setAddingTo(null)
                      if (event.key === 'Enter' && draftName.trim()) {
                        void writeGroup(group, [
                          ...entries,
                          { $typeName: 'metarr.v1.APIKeyEntry', name: draftName.trim(), apiKey: '' },
                        ])
                        setAddingTo(null)
                      }
                    }}
                  />
                  <Button
                    variant="primary"
                    disabled={!draftName.trim()}
                    onClick={() => {
                      void writeGroup(group, [
                        ...entries,
                        { $typeName: 'metarr.v1.APIKeyEntry', name: draftName.trim(), apiKey: '' },
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
      </Space>
    </Card>
  )
}
