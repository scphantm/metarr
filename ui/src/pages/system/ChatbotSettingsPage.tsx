import { useState } from 'react'

import { useChatbotConfig, useUpdateChatbotConfig } from '../../api/queries'
import {
  chatbotProviders,
  type ChatbotConfig,
  type ChatbotMCPConfig,
  type ChatbotProvider,
  type ChatbotProviderKeyConfig,
} from '../../api/types'
import { Button, Card, Row } from '../../components/Card'
import { Spinner } from '../../components/SaveState'
import { PageHeader } from '../../layout/AppShell'

const providerLabels: Record<ChatbotProvider, string> = {
  claude: 'Claude',
  openai: 'ChatGPT',
  gemini: 'Gemini',
  mcp: 'MCP',
}

/*
 * System > Chatbot.
 *
 * Exactly one provider is active (config.provider), but all four providers'
 * settings stay in the draft and round-trip on every save — switching the
 * radio never drops what was typed into the other three.
 */
export function ChatbotSettingsPage() {
  const config = useChatbotConfig()

  if (config.error && !config.data) {
    return (
      <>
        <PageHeader title="Chatbot" />
        <div className="px-6 py-5">
          <p className="rounded border border-red/40 bg-red/10 px-4 py-3 text-sm text-red">
            {config.error instanceof Error
              ? config.error.message
              : String(config.error)}
          </p>
        </div>
      </>
    )
  }

  if (!config.data) {
    return (
      <>
        <PageHeader title="Chatbot" />
        <div className="flex items-center gap-2 px-6 py-5 text-sm text-ink-muted">
          <Spinner />
          Loading configuration…
        </div>
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Chatbot"
        description="One AI provider is connected at a time. Settings for the other three stay saved so switching back doesn't lose anything."
      />
      <div className="flex flex-col gap-5 px-6 py-5">
        <ChatbotForm config={config.data} />
      </div>
    </>
  )
}

function ChatbotForm({ config }: { config: ChatbotConfig }) {
  const update = useUpdateChatbotConfig()
  // config only ever loads once per mount (no live external updater for
  // this document) — draft starts from it and is edited locally until Save.
  const [draft, setDraft] = useState(config)
  const [error, setError] = useState<string | null>(null)

  async function save() {
    setError(null)
    try {
      await update.mutateAsync(draft)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }

  return (
    <Card
      title="Provider"
      description="Which model the chat widget talks to."
      actions={
        <Button variant="primary" onClick={() => void save()} disabled={update.isPending}>
          {update.isPending ? 'Saving…' : 'Save'}
        </Button>
      }
    >
      <Row label="Enabled" hint="Show the chat widget across the app.">
        <label className="inline-flex items-center gap-2 text-sm text-ink-strong">
          <input
            type="checkbox"
            checked={draft.enabled}
            onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}
          />
          Enabled
        </label>
      </Row>

      <Row label="Active provider">
        <ProviderPill
          value={draft.provider}
          onChange={(provider) => setDraft({ ...draft, provider })}
        />
      </Row>

      {draft.provider === 'claude' ? (
        <ProviderKeyFields
          label="Claude"
          value={draft.claude}
          onChange={(claude) => setDraft({ ...draft, claude })}
          modelPlaceholder="claude-sonnet-4-5"
        />
      ) : null}
      {draft.provider === 'openai' ? (
        <ProviderKeyFields
          label="ChatGPT"
          value={draft.openai}
          onChange={(openai) => setDraft({ ...draft, openai })}
          modelPlaceholder="gpt-5"
        />
      ) : null}
      {draft.provider === 'gemini' ? (
        <ProviderKeyFields
          label="Gemini"
          value={draft.gemini}
          onChange={(gemini) => setDraft({ ...draft, gemini })}
          modelPlaceholder="gemini-2.5-pro"
        />
      ) : null}
      {draft.provider === 'mcp' ? (
        <MCPFields value={draft.mcp} onChange={(mcp) => setDraft({ ...draft, mcp })} />
      ) : null}

      {error ? <p className="mt-3 text-xs text-red">{error}</p> : null}
    </Card>
  )
}

function ProviderPill({
  value,
  onChange,
}: {
  value: ChatbotProvider
  onChange: (provider: ChatbotProvider) => void
}) {
  return (
    <div className="flex gap-1 rounded border border-edge bg-surface p-1">
      {chatbotProviders.map((provider) => (
        <button
          key={provider}
          type="button"
          onClick={() => onChange(provider)}
          className={`flex-1 rounded px-2.5 py-1 text-xs transition-colors ${
            value === provider
              ? 'bg-surface-hover text-ink-strong'
              : 'text-ink-muted hover:text-ink-strong'
          }`}
        >
          {providerLabels[provider]}
        </button>
      ))}
    </div>
  )
}

function ProviderKeyFields({
  label,
  value,
  onChange,
  modelPlaceholder,
}: {
  label: string
  value: ChatbotProviderKeyConfig
  onChange: (next: ChatbotProviderKeyConfig) => void
  modelPlaceholder: string
}) {
  return (
    <>
      <Row label={`${label} API key`}>
        <input
          type="password"
          value={value.api_key}
          onChange={(e) => onChange({ ...value, api_key: e.target.value })}
          className="w-full max-w-md rounded border border-edge bg-canvas px-2.5 py-1.5 text-sm text-ink-strong"
        />
      </Row>
      <Row label={`${label} model`}>
        <input
          type="text"
          value={value.model}
          placeholder={modelPlaceholder}
          onChange={(e) => onChange({ ...value, model: e.target.value })}
          className="w-full max-w-md rounded border border-edge bg-canvas px-2.5 py-1.5 text-sm text-ink-strong"
        />
      </Row>
    </>
  )
}

function MCPFields({
  value,
  onChange,
}: {
  value: ChatbotMCPConfig
  onChange: (next: ChatbotMCPConfig) => void
}) {
  return (
    <>
      <Row label="Transport" hint="stdio spawns a local process; http connects to a remote MCP server.">
        <div className="flex gap-1 rounded border border-edge bg-surface p-1">
          {(['stdio', 'http'] as const).map((transport) => (
            <button
              key={transport}
              type="button"
              onClick={() => onChange({ ...value, transport })}
              className={`flex-1 rounded px-2.5 py-1 text-xs transition-colors ${
                value.transport === transport
                  ? 'bg-surface-hover text-ink-strong'
                  : 'text-ink-muted hover:text-ink-strong'
              }`}
            >
              {transport}
            </button>
          ))}
        </div>
      </Row>
      {value.transport === 'stdio' ? (
        <>
          <Row label="Command">
            <input
              type="text"
              value={value.command ?? ''}
              onChange={(e) => onChange({ ...value, command: e.target.value })}
              className="w-full max-w-md rounded border border-edge bg-canvas px-2.5 py-1.5 text-sm text-ink-strong"
            />
          </Row>
          <Row label="Arguments" hint="Space-separated.">
            <input
              type="text"
              value={(value.args ?? []).join(' ')}
              onChange={(e) =>
                onChange({ ...value, args: e.target.value.split(' ').filter(Boolean) })
              }
              className="w-full max-w-md rounded border border-edge bg-canvas px-2.5 py-1.5 text-sm text-ink-strong"
            />
          </Row>
        </>
      ) : (
        <Row label="URL">
          <input
            type="text"
            value={value.url ?? ''}
            onChange={(e) => onChange({ ...value, url: e.target.value })}
            className="w-full max-w-md rounded border border-edge bg-canvas px-2.5 py-1.5 text-sm text-ink-strong"
          />
        </Row>
      )}
      <Row label="Tool name" hint="The MCP tool invoked for a chat completion.">
        <input
          type="text"
          value={value.tool_name}
          onChange={(e) => onChange({ ...value, tool_name: e.target.value })}
          className="w-full max-w-md rounded border border-edge bg-canvas px-2.5 py-1.5 text-sm text-ink-strong"
        />
      </Row>
    </>
  )
}

export function ChatbotSidebar() {
  return (
    <section>
      <h2 className="mb-2 text-xs font-semibold tracking-wide text-ink-muted uppercase">
        How this works
      </h2>
      <div className="rounded border border-edge bg-surface px-3 py-2.5 text-xs leading-relaxed text-ink-muted">
        <p>
          Only one provider is active at a time. Its API key is stored in
          plaintext, the same way Sonarr&apos;s is today.
        </p>
        <p className="mt-2">
          MCP connects Metarr as a client to a model exposed over MCP, local
          or remote — not necessarily &quot;local&quot;, hence the name.
        </p>
      </div>
    </section>
  )
}
