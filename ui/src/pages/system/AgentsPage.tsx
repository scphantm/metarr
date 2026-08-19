import { useState } from 'react'

import { useAgents, useScanDirectories } from '../../api/queries'
import { useSocketStatus } from '../../api/useTopic'
import type { AgentTelemetry, AgentView } from '../../api/types'
import { Button, Card, EmptyState } from '../../components/Card'
import { Spinner } from '../../components/SaveState'
import { PageHeader } from '../../layout/AppShell'
import { AgentConfigureForm } from './AgentConfigureForm'

/*
 * System > Agents.
 *
 * An agent announces itself simply by connecting to Redis, so this screen shows
 * the union of two different things: agents someone has configured, and agents
 * that are currently there. Either can exist without the other, and the
 * difference matters — a configured agent that has gone quiet is a machine to go
 * and check on, while an unconfigured one that has appeared is a machine waiting
 * to be set up.
 */
export function AgentsPage() {
  const agents = useAgents()
  const directories = useScanDirectories()
  const socketStatus = useSocketStatus()

  const [configuring, setConfiguring] = useState<string | null>(null)

  if (agents.error && !agents.data) {
    return (
      <>
        <PageHeader title="Agents" />
        <div className="px-6 py-5">
          <p className="rounded border border-red/40 bg-red/10 px-4 py-3 text-sm text-red">
            {agents.error instanceof Error
              ? agents.error.message
              : String(agents.error)}
          </p>
        </div>
      </>
    )
  }

  if (!agents.data) {
    return (
      <>
        <PageHeader title="Agents" />
        <div className="flex items-center gap-2 px-6 py-5 text-sm text-ink-muted">
          <Spinner />
          Looking for agents…
        </div>
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Agents"
        description="Agents run next to your media and do every filesystem operation. They connect to Redis with nothing but a name; everything else is configured here."
        actions={<ConnectionIndicator status={socketStatus} />}
      />

      <div className="flex flex-col gap-5 px-6 py-5">
        {agents.data.length === 0 ? (
          <EmptyState>
            No agents yet. Start a metarr-agent pointed at this Redis and it
            will appear here within a few seconds.
          </EmptyState>
        ) : null}

        {agents.data.map((agent) => (
          <AgentCard
            key={agent.slug}
            agent={agent}
            scanDirectories={directories.data ?? []}
            configuring={configuring === agent.slug}
            onConfigure={() => setConfiguring(agent.slug)}
            onClose={() => setConfiguring(null)}
          />
        ))}
      </div>
    </>
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

function AgentCard({
  agent,
  scanDirectories,
  configuring,
  onConfigure,
  onClose,
}: {
  agent: AgentView
  scanDirectories: { scanner_slug: string; directory: string }[]
  configuring: boolean
  onConfigure: () => void
  onClose: () => void
}) {
  const needsSetup = agent.online && !agent.configured

  return (
    <Card
      title={agent.display_name || agent.slug}
      description={agent.display_name ? agent.slug : undefined}
      actions={
        <div className="flex items-center gap-2">
          <AgentStatus agent={agent} />
          {configuring ? null : (
            <Button variant={needsSetup ? 'primary' : 'default'} onClick={onConfigure}>
              {agent.configured ? 'Edit' : 'Configure this agent'}
            </Button>
          )}
        </div>
      }
    >
      {needsSetup ? (
        <p className="mb-4 rounded border border-blue/50 bg-blue/10 px-3 py-2 text-sm text-ink">
          This agent has connected but has not been set up yet. Map the
          libraries it can reach to start using it.
        </p>
      ) : null}

      {agent.identity ? <AgentIdentityGrid agent={agent} /> : null}

      {agent.telemetry ? <TelemetryMeters telemetry={agent.telemetry} /> : null}

      {configuring ? (
        <AgentConfigureForm
          agent={agent}
          scanDirectories={scanDirectories}
          onDone={onClose}
        />
      ) : (
        <MappingList agent={agent} />
      )}
    </Card>
  )
}

// Status is a word first and a colour second: an operator scanning a column of
// cards reads the word, and colour alone would carry the whole meaning for
// nobody who cannot separate the hues.
function AgentStatus({ agent }: { agent: AgentView }) {
  if (!agent.online) {
    return (
      <span className="flex items-center gap-1.5 text-xs text-ink-muted">
        <span aria-hidden="true">○</span>
        Offline
      </span>
    )
  }
  if (!agent.configured) {
    return (
      <span className="flex items-center gap-1.5 text-xs text-yellow">
        <span aria-hidden="true">●</span>
        Needs setup
      </span>
    )
  }
  return (
    <span className="flex items-center gap-1.5 text-xs text-green">
      <span aria-hidden="true">●</span>
      Online
    </span>
  )
}

function AgentIdentityGrid({ agent }: { agent: AgentView }) {
  const identity = agent.identity
  if (!identity) return null

  const facts: [string, string][] = [
    ['Host', identity.hostname || '—'],
    ['Address', identity.ip || '—'],
    ['Running as', `${identity.username || 'unknown'} (uid ${identity.uid})`],
    ['Platform', `${identity.os}/${identity.arch}`],
    ['Version', identity.version],
    ['Up since', new Date(identity.started).toLocaleString()],
  ]

  return (
    <dl className="mb-4 grid grid-cols-2 gap-x-6 gap-y-2 sm:grid-cols-3">
      {facts.map(([label, value]) => (
        <div key={label}>
          <dt className="text-xs text-ink-muted">{label}</dt>
          <dd className="truncate text-sm text-ink-strong" title={value}>
            {value}
          </dd>
        </div>
      ))}
    </dl>
  )
}

// CPU and memory are each one value against a known limit, so they are meters —
// a filled track against the same-hue background. A chart of two numbers would
// say less than the numbers do.
function TelemetryMeters({ telemetry }: { telemetry: AgentTelemetry }) {
  const memoryPercent = telemetry.memory_total_bytes
    ? (telemetry.memory_used_bytes / telemetry.memory_total_bytes) * 100
    : 0

  return (
    <div className="mb-4 flex flex-col gap-3">
      <Meter
        label="CPU"
        percent={telemetry.cpu_percent}
        detail={`${telemetry.cpu_percent.toFixed(1)}%`}
      />
      <Meter
        label="Memory"
        percent={memoryPercent}
        detail={`${formatBytes(telemetry.memory_used_bytes)} of ${formatBytes(
          telemetry.memory_total_bytes,
        )}`}
      />
      {(telemetry.gpus ?? []).map((gpu, index) => (
        <Meter
          key={`${gpu.name}-${index}`}
          label={gpu.name || 'GPU'}
          percent={gpu.utilization_percent}
          detail={`${gpu.utilization_percent.toFixed(0)}% · ${formatBytes(
            gpu.memory_used_bytes,
          )} of ${formatBytes(gpu.memory_total_bytes)}`}
        />
      ))}
    </div>
  )
}

function Meter({
  label,
  percent,
  detail,
}: {
  label: string
  percent: number
  detail: string
}) {
  const clamped = Math.max(0, Math.min(100, percent))

  return (
    <div>
      <div className="mb-1 flex items-baseline justify-between gap-3 text-xs">
        <span className="text-ink">{label}</span>
        <span className="text-ink-muted tabular-nums">{detail}</span>
      </div>
      <div
        className="h-1.5 w-full overflow-hidden rounded-full bg-surface-hover"
        role="meter"
        aria-valuenow={Math.round(clamped)}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={label}
      >
        <div
          className="h-full rounded-full bg-blue transition-[width] duration-500 ease-out"
          style={{ width: `${clamped}%` }}
        />
      </div>
    </div>
  )
}

function MappingList({ agent }: { agent: AgentView }) {
  if (agent.mappings.length === 0) {
    return (
      <p className="text-sm text-ink-muted">
        No libraries mapped, so this agent has nothing to scan.
      </p>
    )
  }

  return (
    <div className="flex flex-col gap-1">
      <h3 className="mb-1 text-xs font-semibold tracking-wide text-ink-muted uppercase">
        Mapped libraries
      </h3>
      {agent.mappings.map((mapping) => (
        <div
          key={mapping.scanner_slug}
          className="flex flex-wrap items-baseline gap-x-3 gap-y-1 rounded border border-edge px-3 py-2 text-sm"
        >
          <span className="font-mono text-ink-strong">
            {mapping.scanner_slug}
          </span>
          <span className="font-mono text-xs text-ink-muted">
            {mapping.server_path || '—'}
          </span>
          <span aria-hidden="true" className="text-ink-muted">
            →
          </span>
          <span className="font-mono text-xs text-ink">
            {mapping.agent_path}
          </span>
        </div>
      ))}
    </div>
  )
}

function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const exponent = Math.min(
    units.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1024)),
  )
  const value = bytes / 1024 ** exponent
  return `${value.toFixed(value < 10 && exponent > 0 ? 1 : 0)} ${units[exponent]}`
}

export function AgentsSidebar() {
  return (
    <section>
      <h2 className="mb-2 text-xs font-semibold tracking-wide text-ink-muted uppercase">
        How agents work
      </h2>
      <div className="rounded border border-edge bg-surface px-3 py-2.5 text-xs leading-relaxed text-ink-muted">
        <p>
          An agent is configured locally with only two things: how to reach
          Redis, and its own name. Everything else — which libraries it can see
          and where they live on its machine — is published to it from here.
        </p>
        <p className="mt-2">
          It never connects to the database. Scan results travel back over the
          event bus and are stored under this server's paths, so the library
          reads the same however many agents produced it.
        </p>
      </div>

      <h2 className="mt-6 mb-2 text-xs font-semibold tracking-wide text-ink-muted uppercase">
        Mapping libraries
      </h2>
      <div className="rounded border border-edge bg-surface px-3 py-2.5 text-xs leading-relaxed text-ink-muted">
        <p>
          A mapping says what this machine calls a library you have configured
          under Directory Scanner. Leave one blank when the agent cannot reach
          it — agents sit on different machines and are not expected to see
          everything.
        </p>
        <p className="mt-2">
          Each library belongs to one agent. Two agents scanning the same files
          would each overwrite the other's records.
        </p>
      </div>
    </section>
  )
}
