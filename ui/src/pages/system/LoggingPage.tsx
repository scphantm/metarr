import { useEffect, useRef, useState } from 'react'
import { Badge, Segmented, Typography } from 'antd'

import {
  useAgents,
  useLoggingConfig,
  useLogTail,
  useLogTailStreamStatus,
  useSetAgentLogLevel,
  useUpdateLoggingConfig,
} from '../../api/queries'
import { logLevels, type LogLevel } from '../../api/vocab'
import type { LogTailEntry } from '../../api/types'
import type { LoggingConfig } from '../../gen/metarr/v1/logging_pb'
import { Card, EmptyState } from '../../components/Card'
import { PageError, PageLoading } from '../../components/PageState'
import { PageHeader } from '../../layout/AppShell'
import './LoggingPage.css'

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
  const socketStatus = useLogTailStreamStatus()

  if (logging.error && !logging.data) {
    return (
      <>
        <PageHeader title="Logging" />
        <PageError error={logging.error} />
      </>
    )
  }

  if (!logging.data) {
    return (
      <>
        <PageHeader title="Logging" />
        <PageLoading />
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Logging"
        description="Every log line from the server and every agent ships to one place. Switch verbosity here, live — no restart."
      />

      <div className="page-body">
        <ServerLevelCard config={logging.data} />

        <Card
          title="Agent levels"
          description="One toggle per agent. An agent with no libraries mapped yet can still be switched to debug — useful while working out why it isn't configuring."
        >
          {!agents.data ? (
            <PageLoading>Loading agents…</PageLoading>
          ) : agents.data.length === 0 ? (
            <EmptyState>
              No agents yet. They will appear here once one connects.
            </EmptyState>
          ) : (
            <div className="logging-agent-list">
              {agents.data.map((agent) => (
                <AgentLevelRow
                  key={agent.slug}
                  slug={agent.slug}
                  displayName={agent.displayName}
                  online={agent.online}
                  level={agent.logLevel}
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
    if (level === config.serverLevel) return
    setError(null)
    try {
      await update.mutateAsync({ ...config, serverLevel: level })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause))
    }
  }

  return (
    <Card
      title="Server level"
      description="Applies to metarr-server immediately."
    >
      <LevelPill
        value={config.serverLevel}
        disabled={update.isPending}
        onChange={(level) => void setLevel(level)}
      />
      {error ? (
        <Typography.Text type="danger" style={{ display: 'block', marginTop: 8, fontSize: 12 }}>
          {error}
        </Typography.Text>
      ) : null}
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
    <div className="logging-agent-row">
      <Badge status={online ? 'success' : 'default'} />
      <div className="logging-agent-name">
        <div className="logging-agent-name-primary">{displayName || slug}</div>
        {displayName ? <div className="logging-agent-name-slug">{slug}</div> : null}
      </div>
      <LevelPill
        value={level}
        disabled={setLevel.isPending}
        onChange={(next) => void change(next)}
      />
      {error ? (
        <Typography.Text type="danger" style={{ fontSize: 12 }}>
          {error}
        </Typography.Text>
      ) : null}
    </div>
  )
}

// Same segmented-pill pattern as the theme toggle (layout/Sidebar.tsx).
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
    <Segmented
      value={value}
      disabled={disabled}
      onChange={(next) => onChange(next as LogLevel)}
      options={logLevels.map((level) => ({
        label: level.charAt(0).toUpperCase() + level.slice(1),
        value: level,
      }))}
    />
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
    <div className="logging-pipeline-grid">
      <div>
        <div className="logging-pipeline-label">Sink</div>
        <div className="logging-pipeline-value" style={{ textTransform: 'capitalize' }}>
          {sink || '—'}
        </div>
      </div>
      <div>
        <div className="logging-pipeline-label">Stream</div>
        <div className="logging-pipeline-value editable-field-mono">{stream || '—'}</div>
      </div>
      <div>
        <div className="logging-pipeline-label">Endpoint</div>
        <div className="logging-pipeline-value">
          {endpoint ? (
            <Typography.Link href={endpoint} target="_blank" rel="noreferrer">
              Open in OpenObserve →
            </Typography.Link>
          ) : (
            <Typography.Text type="secondary">not set</Typography.Text>
          )}
        </div>
      </div>
    </div>
  )
}

function ConnectionIndicator({ status }: { status: string }) {
  const label =
    status === 'open' ? 'Live' : status === 'connecting' ? 'Connecting' : 'Stale'
  const badgeStatus =
    status === 'open' ? 'success' : status === 'connecting' ? 'processing' : 'warning'

  return <Badge status={badgeStatus} text={label} />
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
    <div className="logging-live-tail">
      {tail.data.map((entry, index) => (
        <TailLine key={index} entry={entry} />
      ))}
      <div ref={bottomRef} />
    </div>
  )
}

function TailLine({ entry }: { entry: LogTailEntry }) {
  const toneVar =
    entry.level === 'ERROR'
      ? 'var(--color-red)'
      : entry.level === 'WARN'
        ? 'var(--color-yellow)'
        : entry.level === 'DEBUG'
          ? 'var(--ink-muted)'
          : 'var(--ink-body)'

  return (
    <div className="logging-tail-line">
      <span className="logging-tail-time">{new Date(entry.time).toLocaleTimeString()}</span>
      <span className="logging-tail-level" style={{ color: toneVar }}>
        {entry.level}
      </span>
      <span className="logging-tail-source">{entry.source}</span>
      <span className="logging-tail-message">{entry.message}</span>
    </div>
  )
}

export function LoggingSidebar() {
  return (
    <div className="saving-info-sidebar">
      <div>
        <Typography.Title level={5}>How this works</Typography.Title>
        <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
          The server and every agent publish structured log records to Redis; Fluent Bit ships
          them to OpenObserve from there. Neither binary talks to OpenObserve directly.
        </Typography.Paragraph>
        <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
          Logging never blocks the app: if the pipeline falls behind, records are dropped rather
          than slowing anything down, and the process reports how many it dropped.
        </Typography.Paragraph>
      </div>

      <div>
        <Typography.Title level={5}>Switching vendors</Typography.Title>
        <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
          Moving to Splunk or ELK is a Fluent Bit configuration change, not a Metarr one —
          nothing here or in either binary needs to change.
        </Typography.Paragraph>
      </div>
    </div>
  )
}
