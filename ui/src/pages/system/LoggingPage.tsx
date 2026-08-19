import { useEffect, useRef, useState } from 'react'

import {
  useAgents,
  useLoggingConfig,
  useLogTail,
  useSetAgentLogLevel,
  useUpdateLoggingConfig,
} from '../../api/queries'
import { useSocketStatus } from '../../api/useTopic'
import {
  logLevels,
  type LogLevel,
  type LoggingConfig,
  type LogTailEntry,
} from '../../api/types'
import { Card, EmptyState } from '../../components/Card'
import { Spinner } from '../../components/SaveState'
import { PageHeader } from '../../layout/AppShell'

/*
 * System > Logging.
 *
 * Two things live on this screen, and they are different kinds of setting.
 * The level toggles (server, and one per agent) are genuinely wired up here —
 * flipping one takes effect on the running process within moments, no
 * restart. The sink/endpoint/stream fields below them are documentation, not
 * configuration: they describe what Fluent Bit is currently pointed at, and
 * editing them updates that description, but does not reconfigure Fluent Bit
 * itself. Repointing the pipeline at a different vendor is a Fluent Bit
 * config change, deliberately kept out of this screen so that swapping
 * vendors never becomes a Metarr code change.
 */
export function LoggingPage() {
  const logging = useLoggingConfig()
  const agents = useAgents()
  const socketStatus = useSocketStatus()

  if (logging.error && !logging.data) {
    return (
      <>
        <PageHeader title="Logging" />
        <div className="px-6 py-5">
          <p className="rounded border border-red/40 bg-red/10 px-4 py-3 text-sm text-red">
            {logging.error instanceof Error
              ? logging.error.message
              : String(logging.error)}
          </p>
        </div>
      </>
    )
  }

  if (!logging.data) {
    return (
      <>
        <PageHeader title="Logging" />
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
        title="Logging"
        description="Every log line from the server and every agent ships to one place. Switch verbosity here, live — no restart."
      />

      <div className="flex flex-col gap-5 px-6 py-5">
        <ServerLevelCard config={logging.data} />

        <Card
          title="Agent levels"
          description="One toggle per agent. An agent with no libraries mapped yet can still be switched to debug — useful while working out why it isn't configuring."
        >
          {!agents.data ? (
            <div className="flex items-center gap-2 py-2 text-sm text-ink-muted">
              <Spinner />
              Loading agents…
            </div>
          ) : agents.data.length === 0 ? (
            <EmptyState>
              No agents yet. They will appear here once one connects.
            </EmptyState>
          ) : (
            <div className="flex flex-col gap-2">
              {agents.data.map((agent) => (
                <AgentLevelRow
                  key={agent.slug}
                  slug={agent.slug}
                  displayName={agent.display_name}
                  online={agent.online}
                  level={agent.log_level}
                />
              ))}
            </div>
          )}
        </Card>

        <Card
          title="Pipeline"
          description="Where logs are currently shipped. Informational — repointing this at Splunk or ELK is a Fluent Bit config change, not something set here."
        >
          <PipelineInfo
            sink={logging.data.sink}
            endpoint={logging.data.endpoint}
            stream={logging.data.stream}
          />
        </Card>

        <Card
          title="Live tail"
          description="Recent log lines from every process, newest at the bottom."
          actions={<ConnectionIndicator status={socketStatus} />}
        >
          <LiveTail />
        </Card>
      </div>
    </>
  )
}

function ServerLevelCard({ config }: { config: LoggingConfig }) {
  const update = useUpdateLoggingConfig()
  const [error, setError] = useState<string | null>(null)

  async function setLevel(level: LogLevel) {
    if (level === config.server_level) return
    setError(null)
    try {
      await update.mutateAsync({ ...config, server_level: level })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }

  return (
    <Card
      title="Server level"
      description="Applies to metarr-server immediately."
    >
      <div className="flex items-center gap-3">
        <LevelPill
          value={config.server_level}
          disabled={update.isPending}
          onChange={(level) => void setLevel(level)}
        />
      </div>
      {error ? <p className="mt-2 text-xs text-red">{error}</p> : null}
    </Card>
  )
}

function AgentLevelRow({
  slug,
  displayName,
  online,
  level,
}: {
  slug: string
  displayName?: string
  online: boolean
  level: string
}) {
  const setLevel = useSetAgentLogLevel()
  const [error, setError] = useState<string | null>(null)

  async function change(next: LogLevel) {
    if (next === level) return
    setError(null)
    try {
      await setLevel.mutateAsync({ slug, log_level: next })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-3 rounded border border-edge px-3 py-2">
      <span
        aria-hidden="true"
        className={`h-1.5 w-1.5 rounded-full ${online ? 'bg-green' : 'bg-ink-muted'}`}
        title={online ? 'online' : 'offline'}
      />
      <div className="min-w-32 flex-1">
        <div className="font-mono text-sm text-ink-strong">
          {displayName || slug}
        </div>
        {displayName ? (
          <div className="font-mono text-xs text-ink-muted">{slug}</div>
        ) : null}
      </div>
      <LevelPill
        value={level}
        disabled={setLevel.isPending}
        onChange={(next) => void change(next)}
      />
      {error ? <span className="text-xs text-red">{error}</span> : null}
    </div>
  )
}

// The two-button segmented pill used for the dark/light theme toggle
// (layout/Sidebar.tsx) — same pattern, different two values.
function LevelPill({
  value,
  disabled,
  onChange,
}: {
  value: string
  disabled?: boolean
  onChange: (level: LogLevel) => void
}) {
  return (
    <div className="flex gap-1 rounded border border-edge bg-surface p-1">
      {logLevels.map((level) => (
        <button
          key={level}
          type="button"
          disabled={disabled}
          onClick={() => onChange(level)}
          className={`flex-1 rounded px-2.5 py-1 text-xs capitalize transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${
            value === level
              ? 'bg-surface-hover text-ink-strong'
              : 'text-ink-muted hover:text-ink-strong'
          }`}
        >
          {level}
        </button>
      ))}
    </div>
  )
}

function PipelineInfo({
  sink,
  endpoint,
  stream,
}: {
  sink: string
  endpoint: string
  stream: string
}) {
  return (
    <dl className="grid grid-cols-2 gap-x-6 gap-y-2 sm:grid-cols-3">
      <div>
        <dt className="text-xs text-ink-muted">Sink</dt>
        <dd className="text-sm text-ink-strong capitalize">{sink || '—'}</dd>
      </div>
      <div>
        <dt className="text-xs text-ink-muted">Stream</dt>
        <dd className="font-mono text-sm text-ink-strong">{stream || '—'}</dd>
      </div>
      <div>
        <dt className="text-xs text-ink-muted">Endpoint</dt>
        <dd className="text-sm">
          {endpoint ? (
            <a
              href={endpoint}
              target="_blank"
              rel="noreferrer"
              className="text-blue hover:underline"
            >
              Open in OpenObserve →
            </a>
          ) : (
            <span className="text-ink-muted">not set</span>
          )}
        </dd>
      </div>
    </dl>
  )
}

function ConnectionIndicator({ status }: { status: string }) {
  const label =
    status === 'open' ? 'Live' : status === 'connecting' ? 'Connecting' : 'Stale'
  const tone =
    status === 'open'
      ? 'text-green'
      : status === 'connecting'
        ? 'text-yellow'
        : 'text-orange'

  return (
    <span className={`flex items-center gap-1.5 text-xs ${tone}`}>
      <span aria-hidden="true">●</span>
      {label}
    </span>
  )
}

function LiveTail() {
  const tail = useLogTail()
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: 'end' })
  }, [tail.data?.length])

  if (!tail.data || tail.data.length === 0) {
    return <EmptyState>No log lines seen yet.</EmptyState>
  }

  return (
    <div className="max-h-96 overflow-y-auto rounded border border-edge bg-canvas font-mono text-xs">
      {tail.data.map((entry, index) => (
        <TailLine key={index} entry={entry} />
      ))}
      <div ref={bottomRef} />
    </div>
  )
}

function TailLine({ entry }: { entry: LogTailEntry }) {
  const tone =
    entry.level === 'ERROR'
      ? 'text-red'
      : entry.level === 'WARN'
        ? 'text-yellow'
        : entry.level === 'DEBUG'
          ? 'text-ink-muted'
          : 'text-ink'

  return (
    <div className="flex flex-wrap gap-2 border-b border-edge/60 px-3 py-1.5 last:border-b-0">
      <span className="shrink-0 text-ink-muted">
        {new Date(entry.time).toLocaleTimeString()}
      </span>
      <span className={`w-14 shrink-0 ${tone}`}>{entry.level}</span>
      <span className="shrink-0 text-ink-muted">{entry.source}</span>
      <span className="min-w-0 flex-1 text-ink-strong">{entry.message}</span>
    </div>
  )
}

export function LoggingSidebar() {
  return (
    <section>
      <h2 className="mb-2 text-xs font-semibold tracking-wide text-ink-muted uppercase">
        How this works
      </h2>
      <div className="rounded border border-edge bg-surface px-3 py-2.5 text-xs leading-relaxed text-ink-muted">
        <p>
          The server and every agent publish structured log records to Redis;
          Fluent Bit ships them to OpenObserve from there. Neither binary talks
          to OpenObserve directly.
        </p>
        <p className="mt-2">
          Logging never blocks the app: if the pipeline falls behind, records
          are dropped rather than slowing anything down, and the process
          reports how many it dropped.
        </p>
      </div>

      <h2 className="mt-6 mb-2 text-xs font-semibold tracking-wide text-ink-muted uppercase">
        Switching vendors
      </h2>
      <div className="rounded border border-edge bg-surface px-3 py-2.5 text-xs leading-relaxed text-ink-muted">
        Moving to Splunk or ELK is a Fluent Bit configuration change, not a
        Metarr one — nothing here or in either binary needs to change.
      </div>
    </section>
  )
}
