import { useState } from 'react'

import { useDeleteAgent, useUpsertAgent } from '../../api/queries'
import type { AgentView } from '../../api/types'
import { Button } from '../../components/Card'

/*
 * Configuring an agent is one question asked once per library: what does this
 * machine call it?
 *
 * Every configured scan directory is listed, blank by default. A blank row is a
 * deliberate, meaningful answer — this agent cannot reach that library — rather
 * than an unfinished one, which is why nothing here requires filling them all
 * in.
 */
export function AgentConfigureForm({
  agent,
  scanDirectories,
  onDone,
}: {
  agent: AgentView
  scanDirectories: { scanner_slug: string; directory: string }[]
  onDone: () => void
}) {
  const upsert = useUpsertAgent()
  const remove = useDeleteAgent()

  const [displayName, setDisplayName] = useState(agent.display_name ?? '')
  const [paths, setPaths] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      agent.mappings.map((mapping) => [mapping.scanner_slug, mapping.agent_path]),
    ),
  )
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  async function save() {
    setSaving(true)
    setError(null)
    try {
      await upsert.mutateAsync({
        slug: agent.slug,
        display_name: displayName.trim(),
        mappings: Object.entries(paths)
          .filter(([, path]) => path.trim() !== '')
          .map(([scanner_slug, path]) => ({
            scanner_slug,
            agent_path: path.trim(),
          })),
      })
      onDone()
    } catch (cause) {
      // The server explains rejections properly — an already-claimed library
      // names the agent holding it — so its message is shown verbatim.
      setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      setSaving(false)
    }
  }

  async function forget() {
    if (
      !window.confirm(
        `Remove the configuration for "${agent.slug}"? The agent keeps running and will reappear here as unconfigured.`,
      )
    ) {
      return
    }
    setSaving(true)
    try {
      await remove.mutateAsync(agent.slug)
      onDone()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
      setSaving(false)
    }
  }

  return (
    <div className="flex flex-col gap-4 rounded border border-blue/50 px-4 py-3">
      <div className="flex max-w-sm flex-col gap-1">
        <label className="text-xs text-ink-muted" htmlFor={`name-${agent.slug}`}>
          Display name
        </label>
        <input
          id={`name-${agent.slug}`}
          value={displayName}
          placeholder={agent.slug}
          onChange={(event) => setDisplayName(event.target.value)}
          className="rounded border border-edge-strong/40 bg-canvas px-2 py-1 text-sm text-ink-strong focus:border-blue"
        />
      </div>

      <div>
        <h3 className="mb-1 text-xs font-semibold tracking-wide text-ink-muted uppercase">
          Libraries
        </h3>
        <p className="mb-2 text-xs text-ink-muted">
          Enter the path each library has on this agent's machine. Leave one
          blank if the agent cannot reach it.
        </p>

        {scanDirectories.length === 0 ? (
          <p className="text-sm text-ink-muted italic">
            No scan directories configured yet — add one under Directory
            Scanner first.
          </p>
        ) : (
          <div className="flex flex-col gap-2">
            {scanDirectories.map((directory) => (
              <div
                key={directory.scanner_slug}
                className="flex flex-wrap items-center gap-2"
              >
                <div className="w-40 shrink-0">
                  <div className="font-mono text-sm text-ink-strong">
                    {directory.scanner_slug}
                  </div>
                  <div
                    className="truncate font-mono text-xs text-ink-muted"
                    title={directory.directory}
                  >
                    {directory.directory}
                  </div>
                </div>
                <span aria-hidden="true" className="text-ink-muted">
                  →
                </span>
                <input
                  value={paths[directory.scanner_slug] ?? ''}
                  placeholder="not reachable from this agent"
                  aria-label={`Path for ${directory.scanner_slug} on ${agent.slug}`}
                  onChange={(event) =>
                    setPaths((current) => ({
                      ...current,
                      [directory.scanner_slug]: event.target.value,
                    }))
                  }
                  className="min-w-0 flex-1 rounded border border-edge-strong/40 bg-canvas px-2 py-1 font-mono text-sm text-ink-strong focus:border-blue"
                />
              </div>
            ))}
          </div>
        )}
      </div>

      {error ? <p className="text-xs text-red">{error}</p> : null}

      <div className="flex flex-wrap gap-2">
        <Button variant="primary" disabled={saving} onClick={() => void save()}>
          {saving ? 'Saving…' : 'Save'}
        </Button>
        <Button variant="ghost" onClick={onDone}>
          Cancel
        </Button>
        {agent.configured ? (
          <Button variant="danger" disabled={saving} onClick={() => void forget()}>
            Remove agent
          </Button>
        ) : null}
      </div>
    </div>
  )
}
